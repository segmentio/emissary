package emissary

// We're testing the internal util methods in eds so using
// the emissary rather than emissary_test package
import (
	"net"
	"strconv"
	"testing"
	"time"

	"context"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/magiconair/properties/assert"
	"github.com/segmentio/consul-go"
)

func TestEndpointsNotEqual(t *testing.T) {
	var tests = []struct {
		name string
		e1   Endpoint
		e2   Endpoint
	}{
		{
			name: "different host",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test1", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},
		},
		{
			name: "different port",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(81))),
				Tags: make([]string, 0),
			},
		},
		{
			name: "different tags len",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"test "},
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},
		},
		{
			name: "different tags",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"test "},
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"foo "},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, compare(tt.e1, tt.e2), false, "expected endpoints to be not equal")
		})
	}
}

func TestEndpointsEqual(t *testing.T) {
	var tests = []struct {
		name string
		e1   Endpoint
		e2   Endpoint
	}{
		{
			name: "same data",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: make([]string, 0),
			},
		},
		{
			name: "same data with tags",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"test"},
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"test"},
			},
		},
		{
			name: "same data tags diff order",
			e1: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"test", "one"},
			},

			e2: Endpoint{
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Tags: []string{"one", "test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, compare(tt.e1, tt.e2), true, "expected endpoints to be equal")
		})
	}
}

func TestHasAZ(t *testing.T) {
	tags := []string{"foo", "us-west-1a", ""}
	az, ok := hasAz(tags)
	assert.Equal(t, ok, true, "expected az to be found")
	assert.Equal(t, az, "us-west-1a")

	tags = []string{"foo"}
	az, ok = hasAz(tags)
	assert.Equal(t, ok, false, "expected az to be not found")
	assert.Equal(t, az, "")
}

func TestStreamEndpointsUnknownUrl(t *testing.T) {
	s := []struct {
		Service service
	}{
		{Service: service{Address: "192.168.0.1", Port: 4242}},
		{Service: service{Address: "192.168.0.2", Port: 4242}},
		{Service: service{Address: "192.168.0.3", Port: 4242}},
	}
	ss := make([][]struct{ Service service }, 1)
	ss[0] = s
	server, client := newServer(t, ss)
	defer server.Close()

	rslv := ConsulResolver{
		Resolver: &consul.Resolver{
			Client:      client,
			ServiceTags: []string{"A", "B", "C"},
			NodeMeta:    map[string]string{"answer": "42"},
			OnlyPassing: true,
			Cache:       nil,
		},
	}
	eds := NewEdsService(context.Background(), nil, WithConsul(&rslv, 2*time.Second))
	m := &mockEndpointServer{typeUrl: "foo"}
	err := eds.StreamEndpoints(m)
	assert.Equal(t, err.Error(), "unknown TypeUrl foo", "expected unknown TypeUrl")
}

func TestBuildAzMap(t *testing.T) {
	addr1 := serviceAddr(net.JoinHostPort("test1", strconv.Itoa(80)))
	addr2 := serviceAddr(net.JoinHostPort("test2", strconv.Itoa(80)))

	e := []Endpoint{{Addr: addr1}}
	m := buildAzMap(e)
	assert.Equal(t, len(m), 1, "expected map len of 1")
	assert.Equal(t, m["none"][0].Addr, addr1, "expected Addr to be "+string(addr1))

	e = []Endpoint{{Addr: addr1, Tags: []string{"us-east-1a"}}, {Addr: addr2, Tags: []string{"us-east-1c"}}}
	m = buildAzMap(e)
	assert.Equal(t, len(m), 2, "expected map len of 2")
	assert.Equal(t, m["us-east-1a"][0].Addr, addr1, "expected Addr to be "+string(addr1))
	assert.Equal(t, m["us-east-1c"][0].Addr, addr2, "expected Addr to be "+string(addr2))
}

func TestIgnoresRegionOnlyAz(t *testing.T) {
	addr := serviceAddr(net.JoinHostPort("test", strconv.Itoa(80)))
	e := []Endpoint{{Addr: addr, Tags: []string{"us-east-1", "us-east-1c"}}}
	m := buildAzMap(e)
	assert.Equal(t, len(m), 1, "expected map len of 1")
	assert.Equal(t, m["us-east-1c"][0].Addr, addr, "expected Addr to be "+string(addr))
}

func TestStreamEndpoints(t *testing.T) {
	s := []struct {
		Service service
	}{
		{Service: service{Address: "192.168.0.1", Port: 4242}},
		{Service: service{Address: "192.168.0.2", Port: 4242}},
		{Service: service{Address: "192.168.0.3", Port: 4242}},
	}
	ss := make([][]struct{ Service service }, 1)
	ss[0] = s
	server, client := newServer(t, ss)
	defer server.Close()

	rslv := ConsulResolver{
		Resolver: &consul.Resolver{
			Client:      client,
			ServiceTags: []string{"A", "B", "C"},
			NodeMeta:    map[string]string{"answer": "42"},
			OnlyPassing: true,
			Cache:       nil,
		},
	}
	eds := NewEdsService(context.Background(), WithConsul(&rslv, 1*time.Second))
	m := &mockEndpointServer{typeUrl: "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
		resourceNames: []string{"1234"},
		t:             t,
		responseHandler: func(t *testing.T, response *xds.DiscoveryResponse) {
			resource := response.Resources[0]
			cla := &xds.ClusterLoadAssignment{}
			cla.Unmarshal(resource.Value)
			assert.Equal(t, cla.ClusterName, "1234", "expected ClusterName 1234")
			assert.Equal(t, cla.Endpoints[0].LbEndpoints[0].Endpoint.Address.Address, &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: "192.168.0.1",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: uint32(4242),
					},
				},
			})

			assert.Equal(t, cla.Endpoints[1].LbEndpoints[0].Endpoint.Address.Address, &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: "192.168.0.2",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: uint32(4242),
					},
				},
			})

			assert.Equal(t, cla.Endpoints[2].LbEndpoints[0].Endpoint.Address.Address, &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: "192.168.0.3",
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: uint32(4242),
					},
				},
			})
		},
	}
	eds.StreamEndpoints(m)
}
