// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fileutil

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTempPublicKeyFile(t *testing.T) {
	tests := []struct {
		name        string
		publicKey   string
		expectError bool
	}{
		{
			name:        "single-line public key succeeds",
			publicKey:   "single-line-test-public-key",
			expectError: false,
		},
		{
			name:        "multi-line public key succeeds",
			publicKey:   "multi-line\ntest\npublic-key",
			expectError: false,
		},
		{
			name:        "empty public key succeeds",
			publicKey:   "",
			expectError: false,
		},
		{
			name:        "multi-line public key with special characters succeeds",
			publicKey:   "multi-line\ntest\npublic-key\n...# Comment line\nuser@example.com",
			expectError: false,
		},
		{
			name:        "very long public key succeeds",
			publicKey:   strings.Repeat("A", 4096),
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filePath, err := CreateTempPublicKeyFile(tc.publicKey)

			if tc.expectError {
				assert.Empty(t, filePath, "Expected empty file path, got %q", filePath)
				assert.NotNil(t, err, "Expected error, got none")
			} else {
				assert.NotEmpty(t, filePath, "Expected non-empty file path, got %q", filePath)
				defer os.Remove(filePath)

				tempDir := os.TempDir()
				assert.Nil(t, err, "Expected no error, got %v", err)
				assert.True(t, strings.HasPrefix(filePath, tempDir), "Expected file path to be in temp directory, got %q", filePath)
				assert.True(t, strings.HasSuffix(filePath, ".pub"), "Expected file path to have .pub extension, got %q", filePath)
				assert.FileExists(t, filePath, "Expected file at %q to exist, but it does not", filePath)

				// Read file content and verify it matches input
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Errorf("failed to read created file, %q: %v", filePath, err)
				}
				contentString := string(content)
				assert.Equal(t, tc.publicKey, contentString, "File content mismatch. Expected: %q, got: %q", tc.publicKey, contentString)
			}
		})
	}
}

func TestCreateTempPublicKeyFile_FilePermissions(t *testing.T) {
	publicKey := "test-public-key"
	filePath, err := CreateTempPublicKeyFile(publicKey)
	assert.NotEmpty(t, filePath)
	defer os.Remove(filePath)
	assert.Nil(t, err)

	// Check file permissions
	fileInfo, err := os.Stat(filePath)
	assert.Nil(t, err, "Failed to get file info for %q: %v", filePath, err)
	mode := fileInfo.Mode()
	assert.NotEqual(t, mode&0400, 0, "File %q is not readable by owner", filePath)
	assert.NotEqual(t, mode&0200, 0, "File %q is not writable by owner", filePath)
}

func TestCreateTempPublicKeyFile_UniqueFiles(t *testing.T) {
	publicKey := "test-public-key"
	var filePaths []string
	defer func() {
		for _, path := range filePaths {
			os.Remove(path)
		}
	}()

	for i := range 3 {
		filePath, err := CreateTempPublicKeyFile(publicKey)
		assert.Nil(t, err, "CreateTempPublicKeyFile() failed iteration %d: %v", i, err)
		filePaths = append(filePaths, filePath)
	}

	// Verify all paths are unique
	seen := make(map[string]bool)
	for _, filePath := range filePaths {
		assert.False(t, seen[filePath], "Duplicate file path found: %s", filePath)
		seen[filePath] = true
	}
}
