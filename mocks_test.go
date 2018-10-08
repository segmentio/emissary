package emissary

import (
	"context"
	"encoding/json"
	"testing"

	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	consul "github.com/segmentio/consul-go"
	"github.com/segmentio/errors-go"
	"google.golang.org/grpc/metadata"
)

type serviceAddr string

func (serviceAddr) Network() string  { return "" }
func (a serviceAddr) String() string { return string(a) }

type service struct {
	Address string
	Port    int
}

type mockEndpointServer struct {
	t               *testing.T
	typeUrl         string
	resourceNames   []string
	lastResponse    *xds.DiscoveryResponse
	responseHandler func(t *testing.T, response *xds.DiscoveryResponse)
}

func (m *mockEndpointServer) Send(response *xds.DiscoveryResponse) error {
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

func newServerClient(handler func(http.ResponseWriter, *http.Request)) (server *httptest.Server, client *consul.Client) {
	server = httptest.NewServer(http.HandlerFunc(handler))
	client = &consul.Client{
		Address:    server.URL,
		UserAgent:  "test",
		Datacenter: "dc1",
	}
	return
}

func newServer(t *testing.T, services [][]struct{ Service service }) (server *httptest.Server, client *consul.Client) {
	index := 0
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

		json.NewEncoder(res).Encode(services[index])
		index += 1
		return
	})
}
