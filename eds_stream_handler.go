package emissary

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/segmentio/errors-go"
	"github.com/segmentio/stats"
)

// An edsStreamHandler is responsible for servicing a single client on the StreamEndpoints API.
// It keeps a copy of the last endpoint data fetched from consul and sent to Envoy. This is used to determine
// if anything has changed upstream, if so we push an update to Envoy. lastVersion and lastNonce retain
// the respective last pushes of those values to Envoy for the connection. When Envoy receives
// an update it immediately replies, after applying the changes. We use the lastVersion and lastNonce
// to verify that Envoy has received and applied the last change we pushed.
type edsStreamHandler struct {
	lastEndpoints     map[string][]Endpoint
	lastResourceNames map[string]bool
	lastVersion       int
	lastNonce         string
	results           chan EdsResult
	ctx               context.Context
	cancel            context.CancelFunc
}

// Create a new edsStreamHandler
func newEdsStreamHandler() *edsStreamHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &edsStreamHandler{
		lastEndpoints:     make(map[string][]Endpoint),
		lastResourceNames: make(map[string]bool),
		results:           make(chan EdsResult),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// run is the main loop for handling a bi-directional gRPC connection.
// ctx - comes from the main eds service, we watch .Done() to know if we should exit the main run loop
// server - this is our server side stub to the grpc client
// poller - a consulEdsPoller handles querying consul for any resource we're interested in and returns the results
// on this edsStreamHandlers results chan.
//
// If we encounter any error during bi-directional communication with the grpc client, such as Socket read or write error, unexpected
// typeUrl or error to properly parse consul data we will return these errors to the caller
func (e *edsStreamHandler) run(ctx context.Context, server xds.EndpointDiscoveryService_StreamEndpointsServer, poller *edsPoller) error {
	// Close our local edsStreamHandler context when we exit, this tells the handle func from the poller it can exit
	defer e.cancel()
	for {
		// receive from the gRPC server, on error cancel and return
		request, err := server.Recv()
		if err != nil {
			stats.Incr("recv.error")
			return errors.Wrap(err, "recv")
		}

		// If this isn't a request to envoy.api.v2.ClusterLoadAssignment we can't run it and something is very wrong with envoy
		if request.TypeUrl != claURL {
			stats.Incr("type-url.unknown", stats.Tag{Name: "TypeUrl", Value: request.TypeUrl})
			return errors.New(fmt.Sprintf("unknown TypeUrl %s", request.TypeUrl))
		}

		// Check to see if any new services have been added or removed from the list of ResourceNames
		e.updateResources(poller, request)

		// Loop here reading consul data from the results channel. If anything has changed send the results to Envoy,
		// otherwise do nothing and wait for the next batch of results. We need to do this per the Envoy data-plane-api specifications.
		// Otherwise Envoy and Emissary just spin, Envoy receiving the same results and ack'ing and Emissary republishing
		// the same data again and again. This results in a serious performance issue.
	Run:
		for {
			select {
			// watch to see if the main eds server has shutdown, if so exit.
			case <-ctx.Done():
				log.Info("received done in main run loop, exiting")
				return nil
			case r := <-e.results:
				if e.hasChanged(r) {
					err := e.send(server, r, request.TypeUrl)
					stats.Incr("discovery-response.sent")
					if err != nil {
						stats.Incr("discovery-response.sent.error")
						return errors.Wrap(err, "error sending")
					}
					break Run // break out of for run loop and head back to top of run loop to receive response from Envoy
				}
			}
		}
	}
}

// This function inspects the []string of ResourceNames the envoy queries about. ResourcesNames
// correspond to a service within consul. We then build a map of these names. We then look at the previous
// request for resource names and determine if any have been added or remove. If so we add or remove our
// interest in them from the consulEdsPoller
func (e *edsStreamHandler) updateResources(poller *edsPoller, request *xds.DiscoveryRequest) {
	// build a map of resources requested
	newResourceRequests := make(map[string]bool)
	for _, service := range request.ResourceNames {
		newResourceRequests[service] = true
	}

	//check old map, if not in new resource map remove
	for service := range e.lastResourceNames {
		if _, ok := newResourceRequests[service]; !ok {
			poller.removeSubscription(service, e)
		}
	}

	//check new map, if not in old add
	for service := range newResourceRequests {
		if _, ok := e.lastResourceNames[service]; !ok {
			poller.addSubscription(service, e)
		}
	}
}

// Here we're implementing the ResultHandler interface. This function acts as a simple shim to allow
// us to receive a EdsResult from some external source. The select ensures we'll either successfully send
// the update to the edsStreamHandler or return if the edsStreamHandler has exited before we could furnish our results.
func (e *edsStreamHandler) handle(result EdsResult) {
	select {
	case <-e.ctx.Done():
	case e.results <- result:
	}
}

// Send takes the healthy endpoints we received from the poller in a EdsResult and constructs a
// ClusterLoadAssignment protobuf, this is then wrapped in a DiscoveryResponse protobuf and send out grpc.
//
// Before sending we bump the version and create a new Nonce for our response.
// If we encounter any error preparing the response or on send the error is returned to the caller.
func (e *edsStreamHandler) send(server xds.EndpointDiscoveryService_StreamEndpointsServer, results EdsResult, url string) error {
	cla := buildClusterLoadAssignment(results)
	resp, err := buildDiscoveryResponse(url, cla)
	if err != nil {
		return err
	}

	// Bump the version number and set a new nonce
	e.lastVersion = e.lastVersion + 1
	e.lastNonce = string(time.Now().Nanosecond())
	resp.Nonce = e.lastNonce
	resp.VersionInfo = strconv.Itoa(e.lastVersion)

	log.Debug("sending DiscoveryResponse")
	e.lastEndpoints[results.Service] = results.Endpoints
	return server.Send(resp)
}

// Inspect the results of the latest query from consul. Compare with the lastEndpoints cache
// and if we detect a change return true, otherwise return false
func (e *edsStreamHandler) hasChanged(results EdsResult) bool {
	// retrieve the last slice of endpoints cached for our service
	lastEndpoints := e.lastEndpoints[results.Service]
	// If the length differs we have a change
	if len(lastEndpoints) != len(results.Endpoints) {
		stats.Incr("endpoints-count.changed")
		return true
	}

	sort.Slice(lastEndpoints, func(i, j int) bool {
		return bytes.Compare([]byte(lastEndpoints[i].Addr.String()), []byte(lastEndpoints[j].Addr.String())) < 0
	})

	// Now compare consul.Endpoint one by one in each slice
	// at the same index if they are not equal we have a change.
	for i, v := range results.Endpoints {
		if !compare(lastEndpoints[i], v) {
			stats.Incr("endpoints.changed")
			return true
		}
	}

	stats.Incr("endpoints.unchanged")
	// No changes detected
	return false
}
