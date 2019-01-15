package emissary

import (
	"context"

	consul "github.com/segmentio/consul-go"
)

// ConsulResolver implements the Resolver interface.
type ConsulResolver struct {
	Client   *consul.Client
	Resolver *consul.Resolver
}

// Lookup for service in Consul.
func (c *ConsulResolver) Lookup(ctx context.Context, service string) ([]Endpoint, error) {
	consulEndpoints, err := c.Resolver.LookupService(ctx, service)
	if err != nil {
		return nil, err
	}

	var endpoints []Endpoint
	for _, consulEndpoint := range consulEndpoints {
		endpoints = append(endpoints, Endpoint{
			Addr: consulEndpoint.Addr,
			Tags: consulEndpoint.Tags,
		})
	}
	return endpoints, nil
}

func (c *ConsulResolver) Healthy(ctx context.Context) bool {
	var recv string
	err := c.Client.Get(ctx, "/v1/status/leader", nil, &recv)
	if err != nil {
		return false
	}
	return true
}
