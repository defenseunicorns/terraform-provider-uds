// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindTargetLayer(t *testing.T) {
	t.Parallel()

	layerA := ocispec.Descriptor{
		MediaType: "application/vnd.custom.config+yaml",
		Digest:    "sha256:aaaa",
		Size:      100,
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "zarf-config.yaml",
		},
	}
	layerB := ocispec.Descriptor{
		MediaType: "application/vnd.custom.data+json",
		Digest:    "sha256:bbbb",
		Size:      200,
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "values.json",
		},
	}
	layerNoAnnotation := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    "sha256:cccc",
		Size:      50,
	}

	tests := []struct {
		name        string
		layers      []ocispec.Descriptor
		file        string
		mediaType   string
		expected    ocispec.Descriptor
		expectError bool
		errorMsg    string
	}{
		{
			name:     "no filters returns first layer",
			layers:   []ocispec.Descriptor{layerA, layerB},
			expected: layerA,
		},
		{
			name:     "match by file name",
			layers:   []ocispec.Descriptor{layerA, layerB},
			file:     "values.json",
			expected: layerB,
		},
		{
			name:      "match by media type",
			layers:    []ocispec.Descriptor{layerA, layerB},
			mediaType: "application/vnd.custom.data+json",
			expected:  layerB,
		},
		{
			name:      "match by both file and media type",
			layers:    []ocispec.Descriptor{layerA, layerB},
			file:      "zarf-config.yaml",
			mediaType: "application/vnd.custom.config+yaml",
			expected:  layerA,
		},
		{
			name:        "file not found returns error",
			layers:      []ocispec.Descriptor{layerA, layerB},
			file:        "nonexistent.yaml",
			expectError: true,
			errorMsg:    "file \"nonexistent.yaml\" not found",
		},
		{
			name:        "media type not found returns error",
			layers:      []ocispec.Descriptor{layerA, layerB},
			mediaType:   "application/unknown",
			expectError: true,
			errorMsg:    "no layer with media type",
		},
		{
			name:        "file filter with no annotation match",
			layers:      []ocispec.Descriptor{layerNoAnnotation},
			file:        "config.yaml",
			expectError: true,
			errorMsg:    "file \"config.yaml\" not found",
		},
		{
			name:     "no filters with single layer returns it",
			layers:   []ocispec.Descriptor{layerNoAnnotation},
			expected: layerNoAnnotation,
		},
		{
			name:      "media type filter skips non-matching layers",
			layers:    []ocispec.Descriptor{layerA, layerB, layerNoAnnotation},
			mediaType: "application/octet-stream",
			expected:  layerNoAnnotation,
		},
		{
			name:        "file and media type mismatch returns error",
			layers:      []ocispec.Descriptor{layerA, layerB},
			file:        "zarf-config.yaml",
			mediaType:   "application/vnd.custom.data+json",
			expectError: true,
			errorMsg:    "file \"zarf-config.yaml\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := findTargetLayer(tt.layers, tt.file, tt.mediaType)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Digest, result.Digest)
			assert.Equal(t, tt.expected.MediaType, result.MediaType)
		})
	}
}
