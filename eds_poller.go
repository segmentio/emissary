package emissary

import (
	"context"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/segmentio/stats"
)

type edsPoller struct {
	subscriptions map[string]map[resultHandler]struct{} // Service to query in consul and set of interested handlers
	resolver      Resolver                              // our resolver
	ticker        *time.Ticker                          // Ticker for when to poll consul, useful to mock for testing
	mutex         sync.RWMutex                          // RW local for mutating our subscriptions
}

func newEdsPoller(resolver Resolver, ticker *time.Ticker) *edsPoller {
	return &edsPoller{
		subscriptions: make(map[string]map[resultHandler]struct{}),
		resolver:      resolver,
		ticker:        ticker,
	}
}

func (c *edsPoller) addSubscription(service string, handler resultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.subscriptions[service] == nil {
		c.subscriptions[service] = make(map[resultHandler]struct{})
	}
	c.subscriptions[service][handler] = struct{}{}
}

func (c *edsPoller) removeSubscription(service string, handler resultHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	handlers := c.subscriptions[service]
	delete(handlers, handler)
	if len(handlers) == 0 {
		delete(c.subscriptions, service)
	}
}

func (c *edsPoller) removeHandler(handler resultHandler) {
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
func (c *edsPoller) get(service string) []resultHandler {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	handlerMap := c.subscriptions[service]

	// Create a copy and return
	handlers := make([]resultHandler, 0, len(handlerMap))
	for k := range handlerMap {
		handlers = append(handlers, k)
	}
	return handlers
}

// This is our main consul polling loop. On each tick we acquire a read lock on the subscriptions map and
// query the resolver to find the healthy endpoints for each service. We then create a slice of the
// handlers interested in this service and pass them the results.
func (c *edsPoller) pulse(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Info("received done in edsPoller, exiting pulse")
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
					endpoints, err := c.resolver.Lookup(ctx, service)
					if err != nil {
						stats.Incr("resolver.error", stats.Tag{Name: "service", Value: service})
						log.Infof("error querying resolver %s", err)
					}

					log.Infof("resolver found %d endpoint(s)", len(endpoints))

					wg := &sync.WaitGroup{}
					wg.Add(len(handlers))
					// send the results to each of the handlers
					for _, h := range handlers {
						go func(h resultHandler) {
							defer wg.Done()
							h.handle(EdsResult{Service: service, Endpoints: endpoints})
						}(h)
					}

					log.Infof("waiting for consulResultHandlers on %s", service)
					wg.Wait()
				}
			}
		}
	}()
}
