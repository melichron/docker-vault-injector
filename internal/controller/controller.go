// Package controller contains the reconciliation loop. It follows the same
// basic model as controllers in Kubernetes and tools such as Traefik:
// observe current state, calculate desired state, and make the smallest update
// needed to converge the two.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"

	"github.com/vyktory/docker-vault-injector/internal/environment"
	"github.com/vyktory/docker-vault-injector/internal/labels"
	"github.com/vyktory/docker-vault-injector/internal/vaultclient"
)

type Docker interface {
	ListServices(ctx context.Context) ([]swarm.Service, error)
	InspectService(ctx context.Context, id string) (swarm.Service, error)
	UpdateService(ctx context.Context, service swarm.Service) error
	WatchServiceEvents(ctx context.Context) (<-chan events.Message, <-chan error)
}

type Vault interface {
	CurrentVersion(ctx context.Context, mount, path string) (int, error)
	ReadVersion(ctx context.Context, mount, path string, version int) (vaultclient.Secret, error)
}

type Controller struct {
	docker             Docker
	vault              Vault
	logger             *slog.Logger
	pollInterval       time.Duration
	reconcileTimeout   time.Duration
	eventRetryInterval time.Duration
}

func New(
	docker Docker,
	vault Vault,
	logger *slog.Logger,
	pollInterval time.Duration,
	reconcileTimeout time.Duration,
	eventRetryInterval time.Duration,
) *Controller {
	return &Controller{
		docker:             docker,
		vault:              vault,
		logger:             logger,
		pollInterval:       pollInterval,
		reconcileTimeout:   reconcileTimeout,
		eventRetryInterval: eventRetryInterval,
	}
}

// Run performs an initial full reconciliation, then reacts both to Docker
// service events and to a periodic resync. Events provide low latency; polling
// provides correctness after missed events, daemon restarts, or temporary
// network failures.
func (c *Controller) Run(ctx context.Context) error {
	triggers := make(chan string, 128)
	go c.watchEvents(ctx, triggers)

	c.reconcileAllWithLogging(ctx, "startup")

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case serviceID := <-triggers:
			c.reconcileIDWithLogging(ctx, serviceID, "docker-event")
		case <-ticker.C:
			c.reconcileAllWithLogging(ctx, "periodic-resync")
		}
	}
}

// watchEvents reconnects forever. It uses a non-blocking send because a burst
// of service updates can safely collapse into the next periodic reconciliation.
func (c *Controller) watchEvents(ctx context.Context, triggers chan<- string) {
	for ctx.Err() == nil {
		messages, errorsChannel := c.docker.WatchServiceEvents(ctx)
		streamEnded := false

		for !streamEnded {
			select {
			case <-ctx.Done():
				return
			case event, open := <-messages:
				if !open {
					streamEnded = true
					continue
				}
				if event.Actor.ID == "" {
					continue
				}
				select {
				case triggers <- event.Actor.ID:
				default:
					c.logger.Warn("event queue is full; periodic resync will recover", "service_id", event.Actor.ID)
				}
			case err, open := <-errorsChannel:
				if open && err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
					c.logger.Warn("Docker event stream ended", "error", err)
				}
				streamEnded = true
			}
		}

		if !waitForContext(ctx, c.eventRetryInterval) {
			return
		}
	}
}

func (c *Controller) reconcileAllWithLogging(parent context.Context, reason string) {
	listContext, cancelList := context.WithTimeout(parent, c.reconcileTimeout)
	services, err := c.docker.ListServices(listContext)
	cancelList()
	if err != nil {
		c.logger.Error("full reconciliation failed", "reason", reason, "error", err)
		return
	}

	c.logger.Debug("starting full reconciliation", "reason", reason, "services", len(services))
	for _, service := range services {
		// One slow or unavailable Vault path must not consume the timeout budget
		// of every service that follows it.
		reconcileContext, cancelReconcile := context.WithTimeout(parent, c.reconcileTimeout)
		err := c.ReconcileService(reconcileContext, service)
		cancelReconcile()
		if err != nil {
			c.logger.Error("service reconciliation failed",
				"service", service.Spec.Name,
				"service_id", service.ID,
				"reason", reason,
				"error", err,
			)
		}
	}
}

func (c *Controller) reconcileIDWithLogging(parent context.Context, serviceID, reason string) {
	ctx, cancel := context.WithTimeout(parent, c.reconcileTimeout)
	defer cancel()

	service, err := c.docker.InspectService(ctx, serviceID)
	if err != nil {
		// A service may be removed between receiving its event and inspecting it.
		// Treating this as an ordinary error is harmless; the periodic loop does
		// not retain deleted service IDs.
		c.logger.Debug("cannot inspect service from event", "service_id", serviceID, "error", err)
		return
	}
	if err := c.ReconcileService(ctx, service); err != nil {
		c.logger.Error("service reconciliation failed",
			"service", service.Spec.Name,
			"service_id", service.ID,
			"reason", reason,
			"error", err,
		)
	}
}

// ReconcileService is public within the package API primarily to make the core
// behavior easy to test. It performs at most one Docker ServiceUpdate.
func (c *Controller) ReconcileService(ctx context.Context, service swarm.Service) error {
	configuration, err := labels.ParseConfig(service.Spec.Labels)
	if err != nil {
		return err
	}
	if !configuration.Enabled {
		return c.removeManagedEnvironment(ctx, service)
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return fmt.Errorf("service does not use a container task specification")
	}

	appliedVersions, err := labels.ParseAppliedVersions(service.Spec.Labels)
	if err != nil {
		return err
	}
	previouslyManaged, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return err
	}
	desiredNames := configuration.DesiredEnvironmentNames()

	currentVersions := make(map[string]int, len(configuration.Secrets))
	sourceNames := sortedSourceNames(configuration.Secrets)
	for _, sourceName := range sourceNames {
		source := configuration.Secrets[sourceName]
		version, err := c.vault.CurrentVersion(ctx, source.Mount, source.Path)
		if err != nil {
			return fmt.Errorf("secret source %q: %w", sourceName, err)
		}
		currentVersions[sourceName] = version
	}

	currentManagedValues := environment.Select(service.Spec.TaskTemplate.ContainerSpec.Env, desiredNames)
	stateIsCurrent := maps.Equal(currentVersions, appliedVersions) &&
		slices.Equal(desiredNames, previouslyManaged) &&
		environment.Hash(currentManagedValues) == service.Spec.Labels[labels.StateHashLabel]
	if stateIsCurrent {
		return nil
	}

	desiredValues := make(map[string]string, len(desiredNames))
	for _, sourceName := range sourceNames {
		source := configuration.Secrets[sourceName]
		version := currentVersions[sourceName]
		secret, err := c.vault.ReadVersion(ctx, source.Mount, source.Path, version)
		if err != nil {
			return fmt.Errorf("secret source %q: %w", sourceName, err)
		}
		if secret.Version != version {
			return fmt.Errorf("secret source %q: requested version %d but Vault returned version %d", sourceName, version, secret.Version)
		}

		for environmentName, fieldPath := range source.Env {
			value, err := resolveField(secret.Data, fieldPath)
			if err != nil {
				return fmt.Errorf("secret source %q, environment %s: %w", sourceName, environmentName, err)
			}
			desiredValues[environmentName] = value
		}
	}

	updated := cloneServiceForUpdate(service)
	updated.Spec.TaskTemplate.ContainerSpec.Env = environment.Merge(
		service.Spec.TaskTemplate.ContainerSpec.Env,
		previouslyManaged,
		desiredValues,
	)

	versionsJSON, managedJSON, err := labels.MarshalState(currentVersions, desiredNames)
	if err != nil {
		return err
	}
	updated.Spec.Labels[labels.AppliedVersionsLabel] = versionsJSON
	updated.Spec.Labels[labels.ManagedEnvLabel] = managedJSON
	updated.Spec.Labels[labels.StateHashLabel] = environment.Hash(desiredValues)

	if slices.Equal(service.Spec.TaskTemplate.ContainerSpec.Env, updated.Spec.TaskTemplate.ContainerSpec.Env) &&
		maps.Equal(service.Spec.Labels, updated.Spec.Labels) {
		return nil
	}

	if err := c.docker.UpdateService(ctx, updated); err != nil {
		return err
	}
	c.logger.Info("applied Vault environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"versions", versionsJSON,
		"environment_names", desiredNames,
	)
	return nil
}

func (c *Controller) removeManagedEnvironment(ctx context.Context, service swarm.Service) error {
	if !labels.HasControllerState(service.Spec.Labels) {
		return nil
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return fmt.Errorf("cannot clean controller state from a service without a container task specification")
	}

	managed, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return err
	}
	updated := cloneServiceForUpdate(service)
	updated.Spec.TaskTemplate.ContainerSpec.Env = environment.Remove(
		service.Spec.TaskTemplate.ContainerSpec.Env,
		managed,
	)
	delete(updated.Spec.Labels, labels.AppliedVersionsLabel)
	delete(updated.Spec.Labels, labels.ManagedEnvLabel)
	delete(updated.Spec.Labels, labels.StateHashLabel)

	if err := c.docker.UpdateService(ctx, updated); err != nil {
		return err
	}
	c.logger.Info("removed Vault-managed environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"environment_names", managed,
	)
	return nil
}

// cloneServiceForUpdate copies exactly the mutable map, pointer, and slice that
// this controller edits. Every unrelated ServiceSpec field is preserved.
func cloneServiceForUpdate(service swarm.Service) swarm.Service {
	updated := service
	updated.Spec.Labels = maps.Clone(service.Spec.Labels)
	if updated.Spec.Labels == nil {
		updated.Spec.Labels = make(map[string]string)
	}
	if original := service.Spec.TaskTemplate.ContainerSpec; original != nil {
		containerSpec := *original
		containerSpec.Env = slices.Clone(original.Env)
		updated.Spec.TaskTemplate.ContainerSpec = &containerSpec
	}
	return updated
}

func resolveField(data map[string]any, fieldPath string) (string, error) {
	parts := strings.Split(fieldPath, ".")
	var current any = data
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("field path %q contains an empty component", fieldPath)
		}
		object, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field path %q crosses a non-object value at %q", fieldPath, part)
		}
		current, ok = object[part]
		if !ok {
			return "", fmt.Errorf("field path %q does not exist", fieldPath)
		}
	}

	switch value := current.(type) {
	case string:
		return value, nil
	case bool, float32, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode scalar field %q: %w", fieldPath, err)
		}
		return string(encoded), nil
	case nil:
		return "", fmt.Errorf("field path %q resolves to null", fieldPath)
	default:
		return "", fmt.Errorf("field path %q resolves to an object or array; only scalar values can become environment variables", fieldPath)
	}
}

func sortedSourceNames(sources map[string]labels.SecretSource) []string {
	result := make([]string, 0, len(sources))
	for name := range sources {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
