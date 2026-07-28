// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logging

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/stretchr/testify/require"
)

func TestWithPackageContext(t *testing.T) {
	t.Run("includes package operation and namespace", func(t *testing.T) {
		var output bytes.Buffer
		ctx := tflogtest.RootLogger(context.Background(), &output)

		ctx = WithPackageContext(ctx, "create", "podinfo", "demo")
		tflog.Info(ctx, "test event")

		entries, err := tflogtest.MultilineJSONDecode(&output)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, "test event", entries[0]["@message"])
		require.Equal(t, "create", entries[0][fieldOperation])
		require.Equal(t, "podinfo", entries[0][fieldPackage])
		require.Equal(t, "demo", entries[0][fieldNamespace])
	})

	t.Run("omits empty namespace", func(t *testing.T) {
		var output bytes.Buffer
		ctx := tflogtest.RootLogger(context.Background(), &output)

		ctx = WithPackageContext(ctx, "read", "podinfo", "")
		tflog.Info(ctx, "test event")

		entries, err := tflogtest.MultilineJSONDecode(&output)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.NotContains(t, entries[0], fieldNamespace)
	})
}

func TestPackageAndComponentEvents(t *testing.T) {
	duration := 1500 * time.Millisecond

	tests := []struct {
		name     string
		emit     func(context.Context)
		message  string
		fields   map[string]interface{}
		severity string
	}{
		{
			name:     "package started",
			emit:     func(ctx context.Context) { PackageStarted(ctx, "amd64", []string{"init", "app"}) },
			message:  "package started",
			fields:   map[string]interface{}{"architecture": "amd64", "optional_components": "init,app"},
			severity: "info",
		},
		{
			name:     "component started",
			emit:     func(ctx context.Context) { ComponentStarted(ctx, "app") },
			message:  "component started",
			fields:   map[string]interface{}{fieldComponent: "app"},
			severity: "info",
		},
		{
			name:     "package completed",
			emit:     func(ctx context.Context) { PackageCompleted(ctx, duration) },
			message:  "package completed",
			fields:   map[string]interface{}{"duration": duration.String()},
			severity: "info",
		},
		{
			name:     "package failed",
			emit:     func(ctx context.Context) { PackageFailed(ctx, "app", errors.New("command failed")) },
			message:  "package failed",
			fields:   map[string]interface{}{fieldComponent: "app", "error": "command failed"},
			severity: "error",
		},
		{
			name:     "zarf connect command",
			emit:     func(ctx context.Context) { ZarfConnectCommand(ctx, "app", "Open the app") },
			message:  "zarf connect app",
			fields:   map[string]interface{}{"command": "zarf connect app", "description": "Open the app"},
			severity: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := tflogtest.RootLogger(context.Background(), &output)

			tt.emit(ctx)

			entries, err := tflogtest.MultilineJSONDecode(&output)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, tt.message, entries[0]["@message"])
			require.Equal(t, tt.severity, entries[0]["@level"])
			for key, value := range tt.fields {
				require.Equal(t, value, entries[0][key], key)
			}
		})
	}
}
