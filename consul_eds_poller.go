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
	subscriptions map[string]map[consulResultHandler]struct{} // Service to query in consul and set of interested handlers
	resolver      *consul.Resolver                            // our consul-go resolver
	ticker        *time.Ticker                                // Ticker for when to poll consul, useful to mock for testing
	mutex         sync.RWMutex                                // RW local for mutating our subscriptions
}

type consulEdsResult struct {
	service   string
	endpoints []consul.Endpoint
}

func newConsulEdsPoller(resolver *consul.Resolver, ticker *time.Ticker) *consulEdsPoller {
	return &consulEdsPoller{
		subscriptions: make(map[string]map[consulResultHandler]struct{}),
		resolver:      resolver,
		ticker:        ticker,
	}
}

func (c *consulEdsPoller) add(service string, handler consulResultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.subscriptions[service] == nil {
		c.subscriptions[service] = make(map[consulResultHandler]struct{})
	}
	c.subscriptions[service][handler] = struct{}{}
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
				// Make a copy of the services in the subscription map
				c.mutex.RLock()
				services := make([]string, 0, len(c.subscriptions))
				for service := range c.subscriptions {
					services = append(services, service)
				}
				c.mutex.RUnlock()

				// Fetch endpoints from consul for all services we're interested in.
				for _, service := range services {
					handlers := c.get(service)
					stats.Set("eds-poller.subscriptions.count", len(handlers), stats.Tag{Name: "service", Value: service})
					endpoints, err := c.resolver.LookupService(ctx, service)
					if err != nil {
						stats.Incr("resolver.error", stats.Tag{Name: "service", Value: service})
						log.Infof("error querying resolver %s", err)
					}

					wg := &sync.WaitGroup{}
					wg.Add(len(handlers))
					// send the results to each of the handlers
					for _, h := range handlers {
						go func(h consulResultHandler) {
							defer wg.Done()
							h.handle(consulEdsResult{service: service, endpoints: endpoints})
						}(h)
					}

					log.Infof("waiting for consulResultHandlers on %s", service)
					wg.Wait()
				}
			}
		}
	}()
}
