package emissary

import (
	"context"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/segmentio/consul-go"
	"github.com/segmentio/stats"
)

type consulEdsPoller struct {
	subscriptions map[string]map[consulResultHandler]bool // Service to query in consul and set of interested handlers
	resolver      *consul.Resolver                        // our consul-go resolver
	ticker        *time.Ticker                            // Ticker for when to poll consul, useful to mock for testing
	mutex         sync.RWMutex                            // RW local for mutating our subscriptions
}

type consulEdsResult struct {
	service   string
	endpoints []consul.Endpoint
}

func newConsulEdsPoller(resolver *consul.Resolver, ticker *time.Ticker) *consulEdsPoller {
	return &consulEdsPoller{subscriptions: make(map[string]map[consulResultHandler]bool),
		resolver: resolver,
		ticker:   ticker,
		mutex:    sync.RWMutex{},
	}
}

func (c *consulEdsPoller) add(service string, handler consulResultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.subscriptions[service] == nil {
		c.subscriptions[service] = make(map[consulResultHandler]bool)
	}
	c.subscriptions[service][handler] = true
}

func (c *consulEdsPoller) removeSubscription(service string, handler consulResultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	handlers := c.subscriptions[service]
	delete(handlers, handler)
	if len(handlers) == 0 {
		delete(c.subscriptions, service)
	}
}

func (c *consulEdsPoller) removeHandler(handler consulResultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for s, handlers := range c.subscriptions {
		delete(handlers, handler)
		if len(handlers) == 0 {
			delete(c.subscriptions, s)
		}
	}
}

// Primarily here for testing
func (c *consulEdsPoller) get(service string) []consulResultHandler {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	handlerMap := c.subscriptions[service]

	// Create a copy and return
	handlers := make([]consulResultHandler, 0, len(handlerMap))
	for k := range handlerMap {
		handlers = append(handlers, k)
	}
	return handlers
}

// This is our main consul polling loop. On each tick we acquire a read lock on the subscriptions map and
// query the resolver to find the healthy endpoints for each service. We then create a slice of the
// handlers interested in this service and pass them the results.
func (c *consulEdsPoller) pulse(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("received done in consulEdsPoller, exiting pulse")
				return
			case <-c.ticker.C:
				// Fetch endpoints from consul for all services we're interested in.
				c.mutex.RLock()
				results := make(map[string][]consul.Endpoint)
				for service, handlerMap := range c.subscriptions {
					stats.Set("eds-poller.subscriptions.count", len(handlerMap), stats.Tag{Name: "service", Value: service})
					endpoints, err := c.resolver.LookupService(ctx, service)
					if err != nil {
						stats.Incr("resolver.error", stats.Tag{Name: "service", Value: service})
						log.Infof("error querying resolver %s", err)
					}
					results[service] = endpoints

					// Make a new slice and copy out the handlers interested in this service
					handlers := make([]consulResultHandler, 0, len(handlerMap))
					for k := range handlerMap {
						handlers = append(handlers, k)
					}

					// send the results to each of the handlers
					for _, h := range handlers {
						go func(h consulResultHandler) {
							h.handle(consulEdsResult{service: service, endpoints: endpoints})
						}(h)
					}
				}
				c.mutex.RUnlock()
			}
		}
	}()
}
