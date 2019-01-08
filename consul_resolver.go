package emissary

import (
	"context"

	consul "github.com/segmentio/consul-go"
)

// ConsulResolver implements the Resolver interface.
type ConsulResolver struct {
	rslv *consul.Resolver
}

// Lookup for service in Consul.
func (c *ConsulResolver) Lookup(ctx context.Context, service string) ([]Endpoint, error) {
	consulEndpoints, err := c.rslv.LookupService(ctx, service)
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
