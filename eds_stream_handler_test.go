package emissary

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasChanged(t *testing.T) {
	var tests = []struct {
		name     string
		expected bool
		last     map[string][]Endpoint
		new      EdsResult
	}{
		{
			name:     "empty",
			expected: false,
		},
		{
			name:     "new empty",
			expected: false,
			new:      EdsResult{},
		},
		{
			name:     "empty add new",
			expected: true,
			new:      EdsResult{Service: "foo", Endpoints: []Endpoint{{Tags: []string{"test"}}}},
		},
		{
			name:     "same",
			expected: false,
			last: map[string][]Endpoint{
				"foo": {{Tags: []string{"test"}}},
			},
			new: EdsResult{Service: "foo", Endpoints: []Endpoint{{Tags: []string{"test"}}}},
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
