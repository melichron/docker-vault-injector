// Package dockerclient adapts the Moby client to the narrow operations needed
// by the reconciliation controller.
package dockerclient

import (
	"context"
	"fmt"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"
	moby "github.com/moby/moby/client"
)

type Client struct {
	client *moby.Client
}

func NewFromEnvironment() (*Client, error) {
	client, err := moby.New(moby.FromEnv, moby.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) ListServices(ctx context.Context) ([]swarm.Service, error) {
	result, err := c.client.ServiceList(ctx, moby.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Swarm services: %w", err)
	}
	return result.Items, nil
}

func (c *Client) InspectService(ctx context.Context, id string) (swarm.Service, error) {
	result, err := c.client.ServiceInspect(ctx, id, moby.ServiceInspectOptions{})
	if err != nil {
		return swarm.Service{}, fmt.Errorf("inspect Swarm service %s: %w", id, err)
	}
	return result.Service, nil
}

func (c *Client) UpdateService(ctx context.Context, service swarm.Service) error {
	_, err := c.client.ServiceUpdate(ctx, service.ID, moby.ServiceUpdateOptions{
		Version:          service.Version,
		Spec:             service.Spec,
		RegistryAuthFrom: swarm.RegistryAuthFromSpec,
	})
	if err != nil {
		return fmt.Errorf("update Swarm service %s at version %s: %w", service.ID, service.Version.String(), err)
	}
	return nil
}

// WatchServiceEvents creates one event stream. The controller reconnects when
// this stream ends, because daemon restarts and manager failovers are normal.
func (c *Client) WatchServiceEvents(ctx context.Context) (<-chan events.Message, <-chan error) {
	filters := make(moby.Filters).
		Add("type", string(events.ServiceEventType)).
		Add("event", string(events.ActionCreate), string(events.ActionUpdate))
	result := c.client.Events(ctx, moby.EventsListOptions{Filters: filters})
	return result.Messages, result.Err
}
