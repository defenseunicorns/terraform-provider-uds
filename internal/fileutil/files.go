// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fileutil

import (
	"fmt"
	"os"
)

// CreateTempPublicKeyFile creates temporary file containing the public key.
func CreateTempPublicKeyFile(publicKey string) (string, error) {
	tmpFile, err := os.CreateTemp("", "*.pub")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary public key file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(publicKey); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write public key to temporary file: %w", err)
	}

	return tmpFile.Name(), nil
}
