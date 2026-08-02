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

	"github.com/melichron/docker-vault-injector/internal/environment"
	"github.com/melichron/docker-vault-injector/internal/labels"
	"github.com/melichron/docker-vault-injector/internal/retry"
	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
)

type Docker interface {
	ListServices(ctx context.Context) ([]swarm.Service, error)
	InspectService(ctx context.Context, id string) (swarm.Service, error)
	UpdateService(ctx context.Context, service swarm.Service) ([]string, error)
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
	eventRetryMaximum  time.Duration
	status             *statusview.Store
}

func New(
	docker Docker,
	vault Vault,
	logger *slog.Logger,
	pollInterval time.Duration,
	reconcileTimeout time.Duration,
	eventRetryInterval time.Duration,
	eventRetryMaximum time.Duration,
	statusStore *statusview.Store,
) *Controller {
	return &Controller{
		docker:             docker,
		vault:              vault,
		logger:             logger,
		pollInterval:       pollInterval,
		reconcileTimeout:   reconcileTimeout,
		eventRetryInterval: eventRetryInterval,
		eventRetryMaximum:  eventRetryMaximum,
		status:             statusStore,
	}
}

// Run performs an initial full reconciliation, then reacts both to Docker
// service events and to a periodic resync. Events provide low latency; polling
// provides correctness after missed events, daemon restarts, or temporary
// network failures.
func (c *Controller) Run(ctx context.Context) error {
	triggers := make(chan string, 128)
	go c.watchEvents(ctx, triggers)
	c.touchStatus()

	c.reconcileAllWithLogging(ctx, "startup")

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case serviceID := <-triggers:
			c.touchStatus()
			c.reconcileIDWithLogging(ctx, serviceID, "docker-event")
		case <-ticker.C:
			c.touchStatus()
			c.reconcileAllWithLogging(ctx, "periodic-resync")
		}
	}
}

// watchEvents reconnects forever. It uses a non-blocking send because a burst
// of service updates can safely collapse into the next periodic reconciliation.
func (c *Controller) watchEvents(ctx context.Context, triggers chan<- string) {
	reconnectBackoff := retry.NewBackoff(c.eventRetryInterval, c.eventRetryMaximum)
	for ctx.Err() == nil {
		streamStarted := time.Now()
		receivedEvent := false
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
				receivedEvent = true
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

		if receivedEvent || time.Since(streamStarted) >= c.eventRetryInterval {
			reconnectBackoff.Reset()
		}
		delay := reconnectBackoff.Next()
		c.logger.Debug("reconnecting Docker event stream", "retry_after", delay)
		if !waitForContext(ctx, delay) {
			return
		}
	}
}

func (c *Controller) reconcileAllWithLogging(parent context.Context, reason string) {
	c.touchStatus()
	listContext, cancelList := context.WithTimeout(parent, c.reconcileTimeout)
	services, err := c.docker.ListServices(listContext)
	cancelList()
	if err != nil {
		c.logger.Error("full reconciliation failed", "reason", reason, "error", err)
		return
	}

	c.logger.Debug("starting full reconciliation", "reason", reason, "services", len(services))
	activeServiceIDs := make([]string, 0, len(services))
	for _, service := range services {
		activeServiceIDs = append(activeServiceIDs, service.ID)
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
		c.touchStatus()
	}
	if c.status != nil {
		if err := c.status.Prune(activeServiceIDs); err != nil {
			c.logger.Warn("cannot prune status snapshot", "error", err)
		}
	}
	c.touchStatus()
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
func (c *Controller) ReconcileService(ctx context.Context, service swarm.Service) (reconcileErr error) {
	tracked := service.Spec.Labels[labels.EnabledLabel] != "" ||
		labels.HasControllerState(service.Spec.Labels) ||
		hasBootstrapGate(service)
	if !tracked {
		c.removeStatus(service.ID)
		return nil
	}

	report := statusview.Service{
		ID:    service.ID,
		Name:  service.Spec.Name,
		State: statusview.StateReady,
		Gate:  statusview.GateNotUsed,
	}
	if hasBootstrapGate(service) {
		report.Gate = statusview.GateClosed
	}
	defer func() {
		if reconcileErr != nil {
			report.State = statusview.StateError
			report.Error = reconcileErr.Error()
		}
		c.recordStatus(report, reconcileErr == nil)
	}()

	configuration, err := labels.ParseConfig(service.Spec.Labels)
	if err != nil {
		return err
	}
	if !configuration.Enabled {
		report.State = statusview.StateDisabled
		warnings, updated, err := c.removeManagedEnvironment(ctx, service)
		report.Warnings = warnings
		report.UpdateAttempted = updated
		return err
	}
	report.Sources = sortedSourceNames(configuration.Secrets)
	if configuration.BootstrapGate && report.Gate != statusview.GateClosed {
		report.Gate = statusview.GateOpen
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return fmt.Errorf("service does not use a container task specification")
	}

	appliedVersions, err := labels.ParseAppliedVersions(service.Spec.Labels)
	if err != nil {
		return err
	}
	report.Versions = appliedVersions
	previouslyManaged, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return err
	}
	report.EnvironmentNames = previouslyManaged
	configurationHash, err := labels.ConfigHash(configuration)
	if err != nil {
		return err
	}
	bootstrapGatePresent := hasBootstrapGate(service)
	if bootstrapGatePresent && !configuration.BootstrapGate {
		return fmt.Errorf("service contains reserved placement constraint %q but %s is not true", labels.BootstrapGateConstraint, labels.BootstrapGateLabel)
	}
	hasSuccessfulInjectionState := service.Spec.Labels[labels.ConfigHashLabel] == configurationHash &&
		service.Spec.Labels[labels.AppliedVersionsLabel] != "" &&
		service.Spec.Labels[labels.ManagedEnvLabel] != "" &&
		service.Spec.Labels[labels.StateHashLabel] != ""
	if configuration.BootstrapGate &&
		!bootstrapGatePresent &&
		!hasSuccessfulInjectionState {
		report.Gate = statusview.GateMissing
		return fmt.Errorf("%s is true but placement constraint %q is missing; refusing an ungated initial or configuration-changing injection", labels.BootstrapGateLabel, labels.BootstrapGateConstraint)
	}

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
	report.Versions = currentVersions

	// In automatic-import mode the desired environment names live in Vault,
	// not in service labels. The names recorded by the previous successful
	// reconciliation are therefore the only names we can use for this cheap
	// drift check without reading the secret data again.
	currentManagedValues := environment.Select(service.Spec.TaskTemplate.ContainerSpec.Env, previouslyManaged)
	stateIsCurrent := maps.Equal(currentVersions, appliedVersions) &&
		service.Spec.Labels[labels.ConfigHashLabel] == configurationHash &&
		environment.Hash(currentManagedValues) == service.Spec.Labels[labels.StateHashLabel] &&
		!bootstrapGatePresent
	if stateIsCurrent {
		return nil
	}

	desiredValues := make(map[string]string)
	environmentOwners := make(map[string]string)
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

		if len(source.Env) == 0 {
			if len(secret.Data) == 0 {
				return fmt.Errorf("secret source %q: automatic import cannot import an empty Vault document", sourceName)
			}
			fieldNames := sortedFieldNames(secret.Data)
			for _, fieldName := range fieldNames {
				if err := validateDockerEnvironmentName(fieldName); err != nil {
					return fmt.Errorf("secret source %q: top-level field %q: %w", sourceName, fieldName, err)
				}
				value, err := stringifyScalar(secret.Data[fieldName], fmt.Sprintf("top-level field %q", fieldName))
				if err != nil {
					return fmt.Errorf("secret source %q: %w", sourceName, err)
				}
				if err := addDesiredEnvironment(desiredValues, environmentOwners, fieldName, value, sourceName); err != nil {
					return err
				}
			}
			continue
		}

		environmentNames := sortedMappingNames(source.Env)
		for _, environmentName := range environmentNames {
			fieldPath := source.Env[environmentName]
			value, err := resolveField(secret.Data, fieldPath)
			if err != nil {
				return fmt.Errorf("secret source %q, environment %s: %w", sourceName, environmentName, err)
			}
			if err := addDesiredEnvironment(desiredValues, environmentOwners, environmentName, value, sourceName); err != nil {
				return err
			}
		}
	}
	desiredNames := sortedMappingNames(desiredValues)
	report.EnvironmentNames = desiredNames

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
	updated.Spec.Labels[labels.ConfigHashLabel] = configurationHash
	bootstrapGateRemoved := false
	if configuration.BootstrapGate {
		bootstrapGateRemoved = removeBootstrapGate(&updated)
	}

	if slices.Equal(service.Spec.TaskTemplate.ContainerSpec.Env, updated.Spec.TaskTemplate.ContainerSpec.Env) &&
		maps.Equal(service.Spec.Labels, updated.Spec.Labels) &&
		slices.Equal(placementConstraints(service), placementConstraints(updated)) {
		return nil
	}

	warnings, err := c.docker.UpdateService(ctx, updated)
	if err != nil {
		return err
	}
	report.Warnings = warnings
	report.UpdateAttempted = true
	c.logDockerWarnings(service, warnings)
	if bootstrapGateRemoved {
		report.Gate = statusview.GateOpen
	}
	c.logger.Info("applied Vault environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"versions", versionsJSON,
		"environment_names", desiredNames,
		"bootstrap_gate_removed", bootstrapGateRemoved,
	)
	return nil
}

func placementConstraints(service swarm.Service) []string {
	if service.Spec.TaskTemplate.Placement == nil {
		return nil
	}
	return service.Spec.TaskTemplate.Placement.Constraints
}

func (c *Controller) recordStatus(service statusview.Service, successful bool) {
	if c.status == nil {
		return
	}
	if err := c.status.Record(service, successful); err != nil {
		c.logger.Warn("cannot write service status", "service", service.Name, "service_id", service.ID, "error", err)
	}
}

func (c *Controller) removeStatus(serviceID string) {
	if c.status == nil {
		return
	}
	if err := c.status.Remove(serviceID); err != nil {
		c.logger.Warn("cannot remove service status", "service_id", serviceID, "error", err)
	}
}

func (c *Controller) touchStatus() {
	if c.status == nil {
		return
	}
	if err := c.status.Heartbeat(); err != nil {
		c.logger.Warn("cannot write controller heartbeat", "error", err)
	}
}

func (c *Controller) logDockerWarnings(service swarm.Service, warnings []string) {
	for _, warning := range warnings {
		c.logger.Warn("Docker accepted the service update with a warning",
			"service", service.Spec.Name,
			"service_id", service.ID,
			"warning", warning,
		)
	}
}

func (c *Controller) removeManagedEnvironment(ctx context.Context, service swarm.Service) ([]string, bool, error) {
	if !labels.HasControllerState(service.Spec.Labels) {
		return nil, false, nil
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return nil, false, fmt.Errorf("cannot clean controller state from a service without a container task specification")
	}

	managed, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return nil, false, err
	}
	updated := cloneServiceForUpdate(service)
	updated.Spec.TaskTemplate.ContainerSpec.Env = environment.Remove(
		service.Spec.TaskTemplate.ContainerSpec.Env,
		managed,
	)
	delete(updated.Spec.Labels, labels.AppliedVersionsLabel)
	delete(updated.Spec.Labels, labels.ManagedEnvLabel)
	delete(updated.Spec.Labels, labels.StateHashLabel)
	delete(updated.Spec.Labels, labels.ConfigHashLabel)

	warnings, err := c.docker.UpdateService(ctx, updated)
	if err != nil {
		return nil, false, err
	}
	c.logDockerWarnings(service, warnings)
	c.logger.Info("removed Vault-managed environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"environment_names", managed,
	)
	return warnings, true, nil
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
	if original := service.Spec.TaskTemplate.Placement; original != nil {
		placement := *original
		placement.Constraints = slices.Clone(original.Constraints)
		updated.Spec.TaskTemplate.Placement = &placement
	}
	return updated
}

func hasBootstrapGate(service swarm.Service) bool {
	placement := service.Spec.TaskTemplate.Placement
	if placement == nil {
		return false
	}
	for _, constraint := range placement.Constraints {
		if isBootstrapGateConstraint(constraint) {
			return true
		}
	}
	return false
}

// removeBootstrapGate removes every copy of the reserved constraint while
// preserving the order and exact spelling of every operator-owned constraint.
// The service must already have been cloned with cloneServiceForUpdate.
func removeBootstrapGate(service *swarm.Service) bool {
	placement := service.Spec.TaskTemplate.Placement
	if placement == nil {
		return false
	}

	constraints := placement.Constraints[:0]
	removed := false
	for _, constraint := range placement.Constraints {
		if isBootstrapGateConstraint(constraint) {
			removed = true
			continue
		}
		constraints = append(constraints, constraint)
	}
	placement.Constraints = constraints
	return removed
}

func isBootstrapGateConstraint(constraint string) bool {
	// Compose accepts whitespace around the equality operator. Compare a
	// whitespace-free representation so the documented forms behave equally.
	return strings.Join(strings.Fields(constraint), "") == labels.BootstrapGateConstraint
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

	return stringifyScalar(current, fmt.Sprintf("field path %q", fieldPath))
}

// stringifyScalar deliberately refuses objects, arrays and null. Docker
// environment entries are strings, so silently JSON-encoding a structured
// value would hide a likely configuration mistake.
func stringifyScalar(current any, description string) (string, error) {
	switch value := current.(type) {
	case string:
		return value, nil
	case bool, float32, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode scalar %s: %w", description, err)
		}
		return string(encoded), nil
	case nil:
		return "", fmt.Errorf("%s resolves to null", description)
	default:
		return "", fmt.Errorf("%s resolves to an object or array; only scalar values can become environment variables", description)
	}
}

func addDesiredEnvironment(values, owners map[string]string, name, value, sourceName string) error {
	if previousSource, duplicate := owners[name]; duplicate {
		return fmt.Errorf("environment variable %q is produced by both secret source %q and %q", name, previousSource, sourceName)
	}
	owners[name] = sourceName
	values[name] = value
	return nil
}

// Docker represents environment entries as NAME=value. We intentionally do
// not impose shell naming conventions here: dashes, dots and other unusual
// names are an operator choice. Empty names and '=' are structurally
// impossible to represent unambiguously and are the only restrictions.
func validateDockerEnvironmentName(name string) error {
	if name == "" {
		return fmt.Errorf("environment variable name cannot be empty")
	}
	if strings.Contains(name, "=") {
		return fmt.Errorf("environment variable name cannot contain '='")
	}
	return nil
}

func sortedFieldNames(fields map[string]any) []string {
	result := make([]string, 0, len(fields))
	for name := range fields {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortedMappingNames[T any](mapping map[string]T) []string {
	result := make([]string, 0, len(mapping))
	for name := range mapping {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
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
