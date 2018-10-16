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
		e1   consul.Endpoint
		e2   consul.Endpoint
	}{
		{
			name: "different id",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "two",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different host",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test1", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different port",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(81))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different node",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "bar",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different tags len",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test "},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different tags",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test "},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"foo "},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different meta length",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test "},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"foo "},
				Meta: map[string]string{"foo": "bar"},
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different meta",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test "},
				Meta: map[string]string{"foo": "bar"},
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"foo "},
				Meta: map[string]string{"foo": "baz"},
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "different rtt",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test "},
				Meta: map[string]string{"foo": "bar"},
				RTT:  time.Millisecond * 100,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"foo "},
				Meta: map[string]string{"foo": "bar"},
				RTT:  time.Millisecond * 200,
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
		e1   consul.Endpoint
		e2   consul.Endpoint
	}{
		{
			name: "same data",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: make([]string, 0),
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "same data with tags",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test"},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test"},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},
		},
		{
			name: "same data tags diff order",
			e1: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"test", "one"},
				Meta: nil,
				RTT:  time.Millisecond * 200,
			},

			e2: consul.Endpoint{
				ID:   "one",
				Addr: serviceAddr(net.JoinHostPort("test", strconv.Itoa(80))),
				Node: "foo",
				Tags: []string{"one", "test"},
				Meta: nil,
				RTT:  time.Millisecond * 200,
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

	rslv := consul.Resolver{
		Client:      client,
		ServiceTags: []string{"A", "B", "C"},
		NodeMeta:    map[string]string{"answer": "42"},
		OnlyPassing: true,
		Cache:       nil,
	}
	eds := NewEdsService(context.Background(), nil, WithResolver(&rslv))
	m := &mockEndpointServer{typeUrl: "foo"}
	err := eds.StreamEndpoints(m)
	assert.Equal(t, err.Error(), "unknown TypeUrl foo", "expected unknown TypeUrl")
}

func TestBuildAzMap(t *testing.T) {
	e := []consul.Endpoint{{ID: "test"}}
	m := buildAzMap(e)
	assert.Equal(t, len(m), 1, "expected map len of 1")
	assert.Equal(t, m["none"][0].ID, "test", "expected ID of test")

	e = []consul.Endpoint{{ID: "test", Tags: []string{"us-east-1a"}}, {ID: "test", Tags: []string{"us-east-1c"}}}
	m = buildAzMap(e)
	assert.Equal(t, len(m), 2, "expected map len of 2")
	assert.Equal(t, m["us-east-1a"][0].ID, "test", "expected ID of test")
	assert.Equal(t, m["us-east-1c"][0].ID, "test", "expected ID of test")
}

func TestIgnoresRegionOnlyAz(t *testing.T) {
	e := []consul.Endpoint{{ID: "test", Tags: []string{"us-east-1", "us-east-1c"}}}
	m := buildAzMap(e)
	assert.Equal(t, len(m), 1, "expected map len of 1")
	assert.Equal(t, m["us-east-1c"][0].ID, "test", "expected ID of test")
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

	rslv := consul.Resolver{
		Client:      client,
		ServiceTags: []string{"A", "B", "C"},
		NodeMeta:    map[string]string{"answer": "42"},
		OnlyPassing: true,
		Cache:       nil,
	}
	eds := NewEdsService(context.Background(), nil, WithResolver(&rslv))
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
