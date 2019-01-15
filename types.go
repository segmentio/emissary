package emissary

import (
	"context"
	"net"
)

// EdsResult is the output of the EDS sources.
type EdsResult struct {
	Service   string
	Endpoints []Endpoint
}

// Endpoints represents a service endpoint to which Envoy will
// send traffic to.
type Endpoint struct {
	Addr net.Addr
	Tags []string
}

// Resolver to lookup endpoints for a specific service.
type Resolver interface {
	// Lookup for the service or cluster and return a list of Endpoint.
	// Envoy will forward the traffic to those Endpoints.
	Lookup(context.Context, string) ([]Endpoint, error)

	// Return true if the Resolver is healthy.
	Healthy(context.Context) bool
}

type resultHandler interface {
	handle(result EdsResult)
}
