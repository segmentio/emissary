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
	Lookup(context.Context, string) ([]Endpoint, error)
}

type resultHandler interface {
	handle(result EdsResult)
}
