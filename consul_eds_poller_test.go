package emissary

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/consul-go"
	"github.com/stretchr/testify/assert"
)

func TestAddRemove(t *testing.T) {
	p := newConsulEdsPoller(&consul.Resolver{}, time.NewTicker(time.Second))
	esh := &edsStreamHandler{}
	p.add("foo", esh)
	handlers := p.get("foo")
	assert.Equal(t, esh, handlers[0], "expected to find edsStreamHandler")
	p.removeSubscription("foo", esh)
	handlers = p.get("foo")
	assert.Equal(t, 0, len(handlers), "expected empty slice of handlers")
}

func TestRemoveFromSet(t *testing.T) {
	p := newConsulEdsPoller(&consul.Resolver{}, time.NewTicker(time.Second))
	esh := &edsStreamHandler{}
	esh2 := &edsStreamHandler{}
	p.add("foo", esh)
	p.add("foo", esh2)

	handlers := p.get("foo")
	assert.Equal(t, esh, handlers[0], "expected to find edsStreamHandler")
	assert.Equal(t, esh2, handlers[1], "expected to find edsStreamHandler")
	p.removeSubscription("foo", esh)
	handlers = p.get("foo")
	assert.Equal(t, 1, len(handlers), "expected slice len of one")
	assert.Equal(t, esh2, handlers[0], "expected to find second handler")
}

func TestRemoveHandler(t *testing.T) {
	p := newConsulEdsPoller(&consul.Resolver{}, time.NewTicker(time.Second))
	esh := &edsStreamHandler{}
	p.add("foo", esh)
	p.add("bar", esh)

	handlers := p.get("foo")
	assert.Equal(t, 1, len(handlers), "expected slice len of one")
	assert.Equal(t, esh, handlers[0], "expected to find edsStreamHandler")
	handlers = p.get("bar")
	assert.Equal(t, 1, len(handlers), "expected slice len of one")
	assert.Equal(t, esh, handlers[0], "expected to find second handler")

	p.removeHandler(esh)
	handlers = p.get("foo")
	assert.Equal(t, 0, len(handlers), "expected empty slice")
	handlers = p.get("bar")
	assert.Equal(t, 0, len(handlers), "expected empty slice")
}

func TestRemoveHandlerOthersRemaining(t *testing.T) {
	p := newConsulEdsPoller(&consul.Resolver{}, time.NewTicker(time.Second))
	esh := &edsStreamHandler{}
	esh2 := &edsStreamHandler{}
	p.add("foo", esh)
	p.add("bar", esh)
	p.add("bar", esh2)

	handlers := p.get("foo")
	assert.Equal(t, 1, len(handlers), "expected slice len of one")
	assert.Equal(t, esh, handlers[0], "expected to find edsStreamHandler")
	handlers = p.get("bar")
	assert.Equal(t, 2, len(handlers), "expected slice len of one")
	assert.Equal(t, esh, handlers[0], "expected to find first handler")
	assert.Equal(t, esh2, handlers[1], "expected to find second handler")

	p.removeHandler(esh)
	handlers = p.get("foo")
	assert.Equal(t, 0, len(handlers), "expected empty slice")
	handlers = p.get("bar")
	assert.Equal(t, 1, len(handlers), "expected slice len of one")
	assert.Equal(t, esh, handlers[0], "expected to find second handler")
}

func TestPulse(t *testing.T) {
	s1 := []struct {
		Service service
	}{
		{Service: service{Address: "192.168.0.1", Port: 4242}},
		{Service: service{Address: "192.168.0.2", Port: 4242}},
		{Service: service{Address: "192.168.0.3", Port: 4242}},
	}

	// Introduce a change
	s2 := []struct {
		Service service
	}{
		{Service: service{Address: "192.168.0.1", Port: 4242}},
		{Service: service{Address: "192.168.0.2", Port: 4242}},
	}
	ss := make([][]struct{ Service service }, 3)
	ss[0] = s1
	ss[1] = s1
	ss[2] = s2

	_, c := newServer(t, ss)
	r := consul.Resolver{
		Client:      c,
		ServiceTags: []string{"A", "B", "C"},
		NodeMeta:    map[string]string{"answer": "42"},
		OnlyPassing: true,
		Cache:       nil,
	}
	ch := make(chan time.Time)
	tr := &time.Ticker{C: ch}

	cep := consulEdsPoller{subscriptions: make(map[string]map[consulResultHandler]bool),
		resolver: &r,
		ticker:   tr,
		mutex:    sync.RWMutex{},
	}

	h := &mockEdsStreamHandler{results: make(chan consulEdsResult)}
	cep.add("1234", h)

	cep.pulse(context.Background())
	ch <- time.Now()
	firstResult := <-h.results
	assert.Equal(t, 3, len(firstResult.endpoints), "expected 3 results")
	ch <- time.Now()
	secondResult := <-h.results
	assert.Equal(t, 3, len(secondResult.endpoints), "expected 3 results")
	assert.Equal(t, firstResult, secondResult, "expected equal results")
	ch <- time.Now()
	thirdResult := <-h.results
	assert.Equal(t, 2, len(thirdResult.endpoints), "expected 2 results")
	assert.NotEqual(t, secondResult, thirdResult, "expected a change in results")

}

type mockEdsStreamHandler struct {
	results chan consulEdsResult
}

func (m *mockEdsStreamHandler) handle(result consulEdsResult) {
	m.results <- result
}