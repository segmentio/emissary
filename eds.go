package emissary

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	ptypes "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/segmentio/consul-go"
)

var (
	claURL = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
)

// EdsService implements the Envoy xDS EndpointDiscovery service
type EdsService struct {
	consulClient       *consul.Client
	consulPollInterval time.Duration
	rslv               *consul.Resolver
}

// TracerOpt configures a Tracer
type EdsOpt func(t *EdsService)

// Create a new EDS service using consulAddr for fetching
// consul service discovery data
func NewEdsService(client *consul.Client, opts ...EdsOpt) *EdsService {
	return NewEdsServiceWithPollInterval(client, time.Second, opts...)
}

// Create a new EDS service using consulAddr for fetching
// consul service discovery data
func NewEdsServiceWithPollInterval(client *consul.Client, consulPollInterval time.Duration, opts ...EdsOpt) *EdsService {
	eds := &EdsService{consulClient: client, consulPollInterval: consulPollInterval}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(eds)
	}

	return eds
}

// Sets the consul-go resolver to use
func WithResolver(rslv *consul.Resolver) EdsOpt {
	return func(e *EdsService) {
		e.rslv = rslv
	}
}

// Sets the consul-go resolver poll interval
func WithPollInterval(rslv *consul.Resolver) EdsOpt {
	return func(e *EdsService) {
		e.rslv = rslv
	}
}

//////////////////////////////////////// gRPC SUPPORT //////////////////////////////////////////////////////////

// Implementation of the Envoy xDS gRPC Streaming Endpoint for EndpointDiscovery. When we receive
// a new gRPC request create an EdsStreamHandler and call the handle func.
// Handle blocks keep the gRPC connection open for bi-directionally stream updates
func (e *EdsService) StreamEndpoints(server xds.EndpointDiscoveryService_StreamEndpointsServer) error {
	log.Debug("in stream endpoints")
	// TODO add metric
	handler := newEdsStreamHandler(server, e)
	return handler.handle()
}

// An edsStreamHandler if responsible for servicing a single client on the StreamEndpoints API.
// It retains a reference to the server and a consul resolver. Additionally it keeps a copy
// of the last endpoint data fetched from consul and send to Envoy. This is used to determine
// if anything has changed upstream to push updates to Envoy. lastVersion and lastNonce retain
// the respective last pushes of those values to Envoy for the connection. When Envoy receives
// an update it immediately replies after it applies the changes. We use the lastVersion and lastNonce
// to verify that Envoy has received and applied the last change we pushed.
//
// The edsStreamHandler starts a separate child goroutine to routinely query consul. We poll consul
// rather than use watches for performance reasons. The results of the polls are sent on the results channel.
type edsStreamHandler struct {
	server             xds.EndpointDiscoveryService_StreamEndpointsServer
	consulPollInterval time.Duration
	resolver           *consul.Resolver
	lastEndpoints      map[string][]consul.Endpoint
	lastVersion        int
	lastNonce          string
	results            chan map[string][]consul.Endpoint // Had this in a separate type before but probably overkill
}

// Create a new edsStreamHandler
func newEdsStreamHandler(server xds.EndpointDiscoveryService_StreamEndpointsServer, eds *EdsService) edsStreamHandler {
	var rslv *consul.Resolver
	// We allow edsServices to be constructed with a resolver to testing purposes.
	if eds.rslv != nil {
		rslv = eds.rslv
	} else {
		rslv = &consul.Resolver{Client: eds.consulClient}
	}
	return edsStreamHandler{
		resolver:           rslv,
		consulPollInterval: eds.consulPollInterval,
		server:             server,
		lastEndpoints:      make(map[string][]consul.Endpoint),
		results:            make(chan map[string][]consul.Endpoint),
	}
}

// handle is the main loop for handling a gRPC connection.
func (e *edsStreamHandler) handle() error {
	// Create a cancelable context. We need this for the child goroutine we spawn for monitoring consul.
	// If we encounter an error we need to tell that goroutine to exit.
	ctx, cancel := context.WithCancel(context.Background())
	for {
		// receive from the gRPC server, on error cancel and pass to done
		request, err := e.server.Recv()
		if err != nil {
			log.Infof("error in Recv %s", err)
			cancel()
			return err
		}

		// If this isn't a request to envoy.api.v2.ClusterLoadAssignment we can't handle it and something is very wrong with envoy
		if request.TypeUrl != claURL {
			log.Infof("unknown TypeUrl %s", request.TypeUrl)
			cancel()
			return errors.New(fmt.Sprintf("unknown TypeUrl %s", request.TypeUrl))
		}

		// On our first request from the client VersionInfo and ResponseNonce are empty.
		// Subsequent requests from the client will include the current VersionInfo and
		// ResponseNonce
		firstRequest := len(request.VersionInfo) == 0 && len(request.ResponseNonce) == 0

		// If this is our first request we need to start a goroutine to periodically retrieve the endpoint
		// information from consul. Fetch the result from consul and build a map[string][]consul.Endpoint)
		// which is a mapping of service to slice of upstream endpoints.
		if firstRequest {
			go func() {
				for {
					select {
					case <-ctx.Done():
						log.Info("received done in resolver, existing")
						return
					default:
						// Fetch endpoints from consul for all services we're interested in.
						results := make(map[string][]consul.Endpoint)
						for _, cluster := range request.ResourceNames {
							addrs, err := e.resolver.LookupService(context.Background(), cluster)
							if err != nil {
								log.Infof("error querying resolver %s", err)
								cancel()
								return
							}
							results[cluster] = addrs
						}
						e.results <- results
						time.Sleep(e.consulPollInterval)
					}
				}
			}()
		}

		// Loop here reading consul data from the results channel. If this is our first request from this client
		// or anything has changed send the results to Envoy, otherwise wait for the next batch of results.
		// We need to do this per the Envoy data-plane-api specifications. Otherwise Envoy and Emissary just spin,
		// Envoy receiving the same results and ack'ing and Emissary republishing the same data again and again. This
		// results in a serious performance issue.
	Run:
		for {
			select {
			case <-ctx.Done():
				log.Info("received done in main handle loop, exiting")
				return nil
			case r := <-e.results:
				if firstRequest || e.hasChanged(r) {
					err := e.send(r, request.TypeUrl)
					if err != nil {
						cancel()
						log.Infof("error sending %s", err)
						return err
					}
					break Run // break out of for run loop and head back to top of handle loop to receive response from Envoy
				}
			}
		}
	}
}

func (e *edsStreamHandler) send(results map[string][]consul.Endpoint, url string) error {
	resp, err := buildDiscoveryResponse(results, url)
	if err != nil {
		return err
	}
	// Bump the version number and set a new nonce
	e.lastVersion = e.lastVersion + 1
	e.lastNonce = string(time.Now().Nanosecond())
	resp.Nonce = e.lastNonce
	resp.VersionInfo = strconv.Itoa(e.lastVersion)

	log.Debug("sending DiscoveryResponse")
	// TODO add metricNonce:       e.lastNonce,
	e.lastEndpoints = results
	return e.server.Send(resp)
}

// Inspect the results of the latest query from consul. Compare with the lastEndpoints cache
// and if we detect a change return true, otherwise return false
func (e *edsStreamHandler) hasChanged(results map[string][]consul.Endpoint) bool {
	// If number of services changed in this request then we definitely have an update
	if len(results) != len(e.lastEndpoints) {
		log.Info("detected change in requested clusters")
		// TODO metric here
		return true
	}

	// Iterate through each service in the map.
	for service, endpoints := range results {
		// retrieve the last slice of endpoints cached for our service
		lastEndpoints := e.lastEndpoints[service]
		// If the length differs we have a change
		if len(lastEndpoints) != len(endpoints) {
			// TODO metric
			return true
		}

		// The lengths are the same, so now we need to compare
		// the endpoints one-by-one, first sort them.
		sort.Slice(lastEndpoints, func(i, j int) bool {
			return lastEndpoints[i].ID < lastEndpoints[j].ID
		})
		sort.Slice(endpoints, func(i, j int) bool {
			return endpoints[i].ID < endpoints[j].ID
		})

		// Now compare consul.Endpoint one by one in each slice
		// at the same index if they are not equal we have a change.
		for i, v := range endpoints {
			if !compare(lastEndpoints[i], v) {
				// TODO metric
				return true
			}
		}
	}

	// No changes detected
	return false
}

func buildDiscoveryResponse(results map[string][]consul.Endpoint, url string) (*xds.DiscoveryResponse, error) {
	var as []ptypes.Any
	for cluster, endpoints := range results {
		cla := &xds.ClusterLoadAssignment{
			ClusterName: cluster,
		}

		azMap := buildAzMap(endpoints)
		for az, endpoints := range azMap {
			for _, addr := range endpoints {
				a := strings.Split(addr.Addr.String(), ":")
				port, err := strconv.Atoi(a[1])
				if err != nil {
					// TODO Add metric
					log.Infof("error parsing endpoint")
					// Rather than fail here attempt to return some results
					// if possible rather than exiting
					continue
				}
				ee := envoyendpoint.LocalityLbEndpoints{
					Locality: &envoycore.Locality{Zone: az},
					LbEndpoints: []envoyendpoint.LbEndpoint{{
						Endpoint: &envoyendpoint.Endpoint{
							Address: &envoycore.Address{
								Address: &envoycore.Address_SocketAddress{
									SocketAddress: &envoycore.SocketAddress{
										Address: a[0],
										PortSpecifier: &envoycore.SocketAddress_PortValue{
											PortValue: uint32(port),
										},
									},
								},
							},
						},
					},
					},
				}
				cla.Endpoints = append(cla.Endpoints, ee)
			}
		}

		a, err := ptypes.MarshalAny(cla)
		if err != nil {
			return nil, err
		}

		as = append(as, *a)
	}

	return &xds.DiscoveryResponse{
		Resources: as,
		TypeUrl:   url,
	}, nil
}

//////////////////////////////// SHARED UTIL FUNCTIONS ///////////////////////////////////////////////

// Takes a slice of consul.Endpoints and groupBy AZ
func buildAzMap(endoints []consul.Endpoint) map[string][]consul.Endpoint {
	azMap := make(map[string][]consul.Endpoint)
	for _, addr := range endoints {
		az, ok := hasAz(addr.Tags)
		if !ok {
			az = "none"
		}
		azMap[az] = append(azMap[az], addr)
	}

	return azMap
}

// Check a slice of tags to determine if it contains az information.
// If so return the az and true, otherwise return empty string and false
func hasAz(tags []string) (string, bool) {
	for _, t := range tags {
		if strings.HasPrefix(t, "az=") {
			return strings.Split(t, "=")[1], true
		}
	}

	return "", false
}

// Compare two consul.Enpoints for equality.
func compare(e1, e2 consul.Endpoint) bool {
	if &e1 == &e2 {
		return true
	}

	if e1.ID != e2.ID {
		return false
	}

	if e1.Addr != e2.Addr {
		return false
	}

	if e1.Node != e2.Node {
		return false
	}

	sort.Strings(e1.Tags)
	sort.Strings(e2.Tags)

	if len(e1.Tags) != len(e2.Tags) {
		return false
	}

	if len(e1.Meta) != len(e2.Meta) {
		return false
	}

	for i, v := range e1.Tags {
		if e2.Tags[i] != v {
			return false
		}
	}

	for k, v := range e1.Meta {
		if e2.Meta[k] != v {
			return false
		}
	}

	// Not sure if we want to actually compare this, assuming it
	// is updated and populated correctly it would result in more changes than
	// necessary
	if e1.RTT != e2.RTT {
		return false
	}

	return true
}

////////////////////////////////////////// REST ENDPOINT //////////////////////////////////////////////////////////

// FetchEndpoints queries consul to retrieve the latest service endpoint information and returns a DiscoveryResponse and error.
// NOTE: Unlike StreamEndpoints no state is retained on the server to determine if the service endpoints have changed
// therefore every time Envoy queries this endpoint it will receive the latest endpoint information in a DiscoveryResponse.
// Per the xDS protocol spec https://github.com/envoyproxy/data-plane-api/blob/master/XDS_PROTOCOL.md#rest-json-polling-subscriptions
// be careful to not set your polling interval too low or Envoy and this service will spin on this endpoint.
//
// Adding support for only sending a DiscoveryResponse when the underlying endpoints has changed could be added by retaining
// a map[request.Node][map[string][consul.Endpoint] and only only returning a new DiscoveryResponse when the underlying resources
// have changed. One would also need to periodically clean up any resources for nodes no longer calling this endpoint.
func (e *EdsService) FetchEndpoints(ctx context.Context, request *xds.DiscoveryRequest) (*xds.DiscoveryResponse, error) {
	if request.TypeUrl != claURL {
		return nil, errors.New(fmt.Sprintf("unsupported TypeUrl %s", request.TypeUrl))
	}

	results := make(map[string][]consul.Endpoint)
	for _, cluster := range request.ResourceNames {
		addrs, err := e.rslv.LookupService(context.Background(), cluster)
		if err != nil {
			return nil, err
		}
		results[cluster] = addrs
	}

	resp, err := buildDiscoveryResponse(results, request.GetVersionInfo())
	if err != nil {
		return nil, err
	}

	return resp, nil
}
