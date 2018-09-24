package emissary

// We're testing the internal util methods in eds_grpc so using
// the emissary rather than emissary_test package
import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"errors"

	"github.com/apex/log"
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/magiconair/properties/assert"
	"github.com/segmentio/consul-go"
	"google.golang.org/grpc/metadata"
)

type serviceAddr string

func (serviceAddr) Network() string  { return "" }
func (a serviceAddr) String() string { return string(a) }

type mockEndpointServer struct {
	t               *testing.T
	typeUrl         string
	resourceNames   []string
	lastResponse    *xds.DiscoveryResponse
	responseHandler func(t *testing.T, response *xds.DiscoveryResponse)
}

func (m *mockEndpointServer) Send(response *xds.DiscoveryResponse) error {
	log.Infof("Got response +%v", response)
	m.lastResponse = response
	if m.responseHandler != nil {
		m.responseHandler(m.t, response)
	}
	return nil
}

func (m *mockEndpointServer) Recv() (*xds.DiscoveryRequest, error) {
	if m.lastResponse == nil {
		return &xds.DiscoveryRequest{TypeUrl: m.typeUrl, ResourceNames: m.resourceNames}, nil
	}

	return &xds.DiscoveryRequest{
		TypeUrl:       m.typeUrl,
		ResourceNames: m.resourceNames,
		VersionInfo:   m.lastResponse.VersionInfo,
		ResponseNonce: m.lastResponse.Nonce,
	}, errors.New("leaving now")
}

func (m *mockEndpointServer) SetHeader(metadata.MD) error {
	panic("implement me")
}

func (m *mockEndpointServer) SendHeader(metadata.MD) error {
	panic("implement me")
}

func (m *mockEndpointServer) SetTrailer(metadata.MD) {
	panic("implement me")
}

func (m *mockEndpointServer) Context() context.Context {
	panic("implement me")
}

func (me *mockEndpointServer) SendMsg(m interface{}) error {
	panic("implement me")
}

func (me *mockEndpointServer) RecvMsg(m interface{}) error {
	panic("implement me")
}

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
				Node: "bar",
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
				Node: "bar",
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
				Node: "bar",
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
				Node: "bar",
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
				Node: "bar",
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

func TestHasAZ(t *testing.T) {
	tags := []string{"foo", "az=us-west-1a", ""}
	az, ok := hasAz(tags)
	assert.Equal(t, ok, true, "expected az to be found")
	assert.Equal(t, az, "us-west-1a")

	tags = []string{"foo"}
	az, ok = hasAz(tags)
	assert.Equal(t, ok, false, "expected az to be not found")
	assert.Equal(t, az, "")
}

func TestStreamEndpointsUnknownUrl(t *testing.T) {
	server, client := newServer(t)
	defer server.Close()

	rslv := consul.Resolver{
		Client:      client,
		ServiceTags: []string{"A", "B", "C"},
		NodeMeta:    map[string]string{"answer": "42"},
		OnlyPassing: true,
		Cache:       nil,
	}
	eds := NewEdsService(nil, WithResolver(&rslv))
	m := &mockEndpointServer{typeUrl: "foo"}
	err := eds.StreamEndpoints(m)
	assert.Equal(t, err.Error(), "unknown TypeUrl foo", "expected unknown TypeUrl")
}

func TestStreamEndpoints(t *testing.T) {
	server, client := newServer(t)
	defer server.Close()

	rslv := consul.Resolver{
		Client:      client,
		ServiceTags: []string{"A", "B", "C"},
		NodeMeta:    map[string]string{"answer": "42"},
		OnlyPassing: true,
		Cache:       nil,
	}
	eds := NewEdsService(nil, WithResolver(&rslv))
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

func newServerClient(handler func(http.ResponseWriter, *http.Request)) (server *httptest.Server, client *consul.Client) {
	server = httptest.NewServer(http.HandlerFunc(handler))
	client = &consul.Client{
		Address:    server.URL,
		UserAgent:  "test",
		Datacenter: "dc1",
	}
	return
}

func newServer(t *testing.T) (server *httptest.Server, client *consul.Client) {
	return newServerClient(func(res http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			t.Error("bad method:", req.Method)
		}

		if req.URL.Path != "/v1/health/service/1234" {
			t.Error("bad URL path:", req.URL.Path)
		}

		foundQuery := req.URL.Query()
		expectQuery := url.Values{
			"passing":   {""},
			"stale":     {""},
			"dc":        {"dc1"},
			"tag":       {"A", "B", "C"},
			"node-meta": {"answer:42"},
		}
		if !reflect.DeepEqual(foundQuery, expectQuery) {
			t.Error("bad URL query:")
			t.Logf("expected: %#v", expectQuery)
			t.Logf("found:    %#v", foundQuery)
		}

		type service struct {
			Address string
			Port    int
		}
		json.NewEncoder(res).Encode([]struct {
			Service service
		}{
			{Service: service{Address: "192.168.0.1", Port: 4242}},
			{Service: service{Address: "192.168.0.2", Port: 4242}},
			{Service: service{Address: "192.168.0.3", Port: 4242}},
		})
		return
	})
}
