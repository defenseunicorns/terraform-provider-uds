// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseZarfConfigSetVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		expected    map[string]string
		expectError bool
	}{
		{
			name:     "empty string returns nil",
			content:  "",
			expected: nil,
		},
		{
			name: "valid config with deploy set variables",
			content: `package:
  deploy:
    set:
      REGISTRY_URL: "registry.example.com"
      REGISTRY_USERNAME: "admin"
      REGISTRY_PASSWORD: "secret123"
`,
			expected: map[string]string{
				"REGISTRY_URL":      "registry.example.com",
				"REGISTRY_USERNAME": "admin",
				"REGISTRY_PASSWORD": "secret123",
			},
		},
		{
			name: "config with no set variables returns nil map",
			content: `package:
  deploy: {}
`,
			expected: nil,
		},
		{
			name: "config with empty set returns empty map",
			content: `package:
  deploy:
    set: {}
`,
			expected: map[string]string{},
		},
		{
			name: "config with only root-level keys ignores them",
			content: `architecture: amd64
plain_http: true
`,
			expected: nil,
		},
		{
			name: "config with single variable",
			content: `package:
  deploy:
    set:
      MY_VAR: "my-value"
`,
			expected: map[string]string{
				"MY_VAR": "my-value",
			},
		},
		{
			name:        "invalid YAML returns error",
			content:     "not: valid: yaml: [[[",
			expectError: true,
		},
		{
			name: "config with extra fields is parsed without error",
			content: `package:
  deploy:
    set:
      FOO: bar
    timeout: 30m
    components: "comp1,comp2"
  create:
    set:
      BUILD_VAR: build-value
`,
			expected: map[string]string{
				"FOO": "bar",
			},
		},
		{
			name: "config with multiline value",
			content: `package:
  deploy:
    set:
      MULTI_LINE: |-
        line1
        line2
        line3
`,
			expected: map[string]string{
				"MULTI_LINE": "line1\nline2\nline3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseZarfConfigSetVariables(tt.content)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
