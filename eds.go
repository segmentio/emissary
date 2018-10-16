package emissary

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	ptypes "github.com/gogo/protobuf/types"
	"github.com/segmentio/consul-go"
	"github.com/segmentio/errors-go"
	"github.com/segmentio/stats"
)

var (
	claURL = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
)

// EdsService implements the Envoy xDS EndpointDiscovery service
type EdsService struct {
	consulEndpointPoller *consulEdsPoller // our consul poller that handles subscriptions for endpoint info from grpc clients
	rslv                 *consul.Resolver // our consul resolver
	ctx                  context.Context  // our root context
}

// TracerOpt configures a Tracer
type EdsOpt func(t *EdsService)

// Create a new EDS service using consulAddr for fetching
// consul service discovery data
func NewEdsService(ctx context.Context, client *consul.Client, opts ...EdsOpt) *EdsService {
	return NewEdsServiceWithPollInterval(ctx, client, time.Second, opts...)
}

// Create a new EDS service using consul client to fetch consul service endpoints
func NewEdsServiceWithPollInterval(ctx context.Context, client *consul.Client, consulPollInterval time.Duration, opts ...EdsOpt) *EdsService {
	rslv := consul.Resolver{Client: client}
	poller := newConsulEdsPoller(&rslv, time.NewTicker(consulPollInterval))
	eds := &EdsService{consulEndpointPoller: poller, rslv: &rslv, ctx: ctx}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(eds)
	}

	poller.pulse(ctx)
	return eds
}

// Sets the consul-go resolver to use
func WithResolver(rslv *consul.Resolver) EdsOpt {
	return func(e *EdsService) {
		e.rslv = rslv
		e.consulEndpointPoller.resolver = rslv
	}
}

//////////////////////////////////////// gRPC SUPPORT //////////////////////////////////////////////////////////

// Implementation of the Envoy xDS gRPC Streaming Endpoint for EndpointDiscovery. When we receive
// a new gRPC request create an EdsStreamHandler and call the run func.
// Handle blocks keeping the gRPC connection open for bi-directional stream updates
func (e *EdsService) StreamEndpoints(server xds.EndpointDiscoveryService_StreamEndpointsServer) error {
	log.Debug("in stream endpoints")
	stats.Incr("stream-endpoints.connections.new")
	handler := newEdsStreamHandler()
	err := handler.run(e.ctx, server, e.consulEndpointPoller)
	if err != nil {
		log.Infof("error in handler %s", err)
		stats.Incr("stream-endpoints.handler.error")
	}
	stats.Incr("stream-endpoints.connections.closed")
	e.consulEndpointPoller.removeHandler(handler)
	return err
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
		return nil, errors.Errorf("unsupported TypeUrl %s", request.TypeUrl)
	}

	results := make([]consulEdsResult, len(request.ResourceNames))
	for _, cluster := range request.ResourceNames {
		endpoints, err := e.rslv.LookupService(context.Background(), cluster)
		if err != nil {
			return nil, errors.Wrap(err, "lookup service")
		}
		results = append(results, consulEdsResult{service: cluster, endpoints: endpoints})
	}

	clas := make([]*xds.ClusterLoadAssignment, len(results))
	for _, result := range results {
		clas = append(clas, buildClusterLoadAssignment(result))
	}

	resp, err := buildDiscoveryResponse(request.TypeUrl, clas...)
	if err != nil {
		return nil, errors.Wrap(err, "error building DiscoveryResponse")
	}

	return resp, nil
}

// Takes a variadic number of ClusterLoadAssignments and generates a DiscoveryResponse
func buildDiscoveryResponse(url string, clas ...*xds.ClusterLoadAssignment) (*xds.DiscoveryResponse, error) {
	var as []ptypes.Any
	for _, cla := range clas {
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

// Converts a consulEdsResult (list of healthy endpoints for a service in consul) to a ClusterLoadAssignment,
// which is a protobuf generated payload the envoy client expects over grpc.
func buildClusterLoadAssignment(results consulEdsResult) *xds.ClusterLoadAssignment {
	cla := &xds.ClusterLoadAssignment{
		ClusterName: results.service,
	}

	azMap := buildAzMap(results.endpoints)
	for az, endpoints := range azMap {
		for _, addr := range endpoints {
			a := strings.Split(addr.Addr.String(), ":")
			port, err := strconv.Atoi(a[1])
			if err != nil {
				stats.Incr("endpoint.parse.error")
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

	return cla
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
		if strings.HasPrefix(t, "us-") {
			return t, true
		}
	}

	return "", false
}

// Compare two consul.Endpoints for equality.
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
