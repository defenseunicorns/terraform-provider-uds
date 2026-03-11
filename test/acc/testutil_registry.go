// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory" // in-memory storage driver
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// setupInMemoryRegistry starts an in-memory OCI registry on a random available port
// and returns the "localhost:<port>" address. The registry is cleaned up when the test ends.
func setupInMemoryRegistry(ctx context.Context, t *testing.T, port int) string {
	t.Helper()
	config := &configuration.Configuration{}
	config.HTTP.Addr = fmt.Sprintf(":%d", port)
	config.Log.AccessLog.Disabled = true
	config.Log.Level = "error"
	logrus.SetOutput(io.Discard)
	config.HTTP.DrainTimeout = 10 * time.Second
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]interface{}{}}
	reg, err := registry.NewRegistry(ctx, config)
	require.NoError(t, err)
	//nolint:errcheck // registry server error is not actionable in tests
	go reg.ListenAndServe()
	return fmt.Sprintf("localhost:%d", port)
}
