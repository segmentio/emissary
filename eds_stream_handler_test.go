package emissary

import (
	"fmt"
	"testing"

	"github.com/segmentio/consul-go"
	"github.com/stretchr/testify/assert"
)

func TestHasChanged(t *testing.T) {
	var tests = []struct {
		name     string
		expected bool
		last     map[string][]consul.Endpoint
		new      consulEdsResult
	}{
		{
			name:     "empty",
			expected: false,
		},
		{
			name:     "new empty",
			expected: false,
			new:      consulEdsResult{},
		},
		{
			name:     "empty add new",
			expected: true,
			new:      consulEdsResult{service: "foo", endpoints: []consul.Endpoint{{ID: "test"}}},
		},
		{
			name:     "same",
			expected: false,
			last: map[string][]consul.Endpoint{
				"foo": {{ID: "test"}},
			},
			new: consulEdsResult{service: "foo", endpoints: []consul.Endpoint{{ID: "test"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esh := &edsStreamHandler{}
			if tt.last != nil {
				esh.lastEndpoints = tt.last
			}

			assert.Equal(t, esh.hasChanged(tt.new), tt.expected, fmt.Sprintf("expected  %v", tt.expected))
		})
	}
}
