// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/stretchr/testify/require"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	"github.com/zarf-dev/zarf/src/pkg/utils/exec"
)

// Capture policy and buffer bounds.

func TestOutputBufferEnforcesRecordAndByteLimits(t *testing.T) {
	tests := []struct {
		name       string
		maxRecords int
		maxBytes   int
		messages   []string
		accepted   []bool
		wantBytes  int
	}{
		{
			name:       "record limit",
			maxRecords: 2,
			maxBytes:   100,
			messages:   []string{"one", "two", "three"},
			accepted:   []bool{true, true, false},
			wantBytes:  len("one") + len("two"),
		},
		{
			name:       "byte limit",
			maxRecords: 100,
			maxBytes:   5,
			messages:   []string{"one", "three"},
			accepted:   []bool{true, false},
			wantBytes:  len("one"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := outputBuffer{maxRecords: tt.maxRecords, maxBytes: tt.maxBytes}
			for i, message := range tt.messages {
				require.Equal(t, tt.accepted[i], buffer.tryAppend(message))
			}
			require.Equal(t, tt.wantBytes, buffer.bytes)
		})
	}
}

func TestIsZarfOutputLevelForCapture(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		want  bool
	}{
		{name: "info", level: slog.LevelInfo, want: true},
		{name: "warn", level: slog.LevelWarn, want: false},
		{name: "error", level: slog.LevelError, want: true},
		{name: "debug", level: slog.LevelDebug, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isZarfOutputLevel(tt.level))
		})
	}
}

func TestZarfHandlerShouldCaptureOutput(t *testing.T) {
	tests := []struct {
		name           string
		handlerAttrs   []zarfAttr
		hasRecordAttrs bool
		level          slog.Level
		want           bool
	}{
		{name: "bare info record", level: slog.LevelInfo, want: true},
		{name: "bare error record", level: slog.LevelError, want: true},
		{name: "handler attributes", handlerAttrs: []zarfAttr{{attr: slog.String("component", "app")}}, level: slog.LevelInfo},
		{name: "record attributes", hasRecordAttrs: true, level: slog.LevelInfo},
		{name: "debug record", level: slog.LevelDebug},
		{name: "warn record", level: slog.LevelWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &zarfHandler{attrs: tt.handlerAttrs}
			record := slog.NewRecord(time.Time{}, tt.level, "message", 0)
			require.Equal(t, tt.want, handler.shouldCaptureZarfOutput(record, tt.hasRecordAttrs))
		})
	}
}

// Forwarding and structured fields.

func TestZarfSeverityAndFields(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		expected string
	}{
		{name: "trace", level: slog.Level(-8), expected: "trace"},
		{name: "debug", level: slog.LevelDebug, expected: "debug"},
		{name: "info", level: slog.LevelInfo, expected: "info"},
		{name: "warn", level: slog.LevelWarn, expected: "warn"},
		{name: "error", level: slog.LevelError, expected: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := tflogtest.RootLogger(context.Background(), &output)
			ctx = WithPackageContext(ctx, "create", "podinfo", "demo")
			zarfCtx := WithZarfLogger(ctx)

			logger.From(zarfCtx).Log(zarfCtx, tt.level, "zarf event",
				slog.String("package", "podinfo"),
				slog.String("component", "app"),
				slog.String("cmd", "echo hello"),
				slog.Duration("duration", 1500*time.Millisecond),
				slog.String("stream", "stdout"),
			)

			entries, err := tflogtest.MultilineJSONDecode(&output)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, "zarf event", entries[0]["@message"])
			require.Equal(t, tt.expected, entries[0]["@level"])
			require.Equal(t, "provider.zarf", entries[0]["@module"])
			require.Equal(t, "podinfo", entries[0]["package"])
			require.Equal(t, "app", entries[0]["component"])
			require.Equal(t, "echo hello", entries[0]["cmd"])
			require.Equal(t, "1.5s", entries[0]["duration"])
			require.Equal(t, "stdout", entries[0]["stream"])
		})
	}
}

func TestZarfPreservesGroupedAttributes(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	handler := NewZarfHandler(ctx)
	log := slog.New(handler).With(slog.String("root", "preserved"))

	log.WithGroup("command").Info("grouped event",
		slog.String("name", "deploy"),
		slog.Group("details", slog.String("stream", "stdout")),
	)

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "preserved", entries[0]["root"])
	require.Equal(t, "deploy", entries[0]["command.name"])
	require.Equal(t, "stdout", entries[0]["command.details.stream"])
}

func TestZarfPreservesProviderContextAndUsesSubsystem(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithPackageContext(ctx, "read", "podinfo", "demo")
	ctx = tflog.NewSubsystem(ctx, "zarf")
	ctx = WithZarfLogger(ctx)

	logger.From(ctx).Info("context event")

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "provider.zarf", entries[0]["@module"])
	require.Equal(t, "read", entries[0][fieldOperation])
	require.Equal(t, "podinfo", entries[0][fieldPackage])
	require.Equal(t, "demo", entries[0][fieldNamespace])
}

// Zarf command output forwarding.

func TestZarfLogWriterForwardsNonMutedCommandOutput(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithZarfLogger(ctx)

	writer := &logger.LogWriter{Logger: logger.From(ctx), Level: logger.Info}
	_, err := writer.Write([]byte("Zarf command output\n"))
	require.NoError(t, err)

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "Zarf command output", entries[0]["@message"])
	require.Equal(t, "info", entries[0]["@level"])
}

func TestZarfLogWriterForwardsStdoutAndStderrAtTheirSeverities(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithZarfLogger(ctx)

	for _, level := range []logger.Level{logger.Info, logger.Error} {
		writer := &logger.LogWriter{Logger: logger.From(ctx), Level: level}
		_, err := writer.Write([]byte("Zarf command output\n"))
		require.NoError(t, err)
	}

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "info", entries[0]["@level"])
	require.Equal(t, "error", entries[1]["@level"])
	for _, entry := range entries {
		require.Equal(t, "Zarf command output", entry["@message"])
	}
}

func TestZarfMutedOutputDoesNotUseLogWriter(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithZarfLogger(ctx)

	previousPreferLogger := zarfConfig.CommonOptions.PreferLogger
	zarfConfig.CommonOptions.PreferLogger = true
	t.Cleanup(func() {
		zarfConfig.CommonOptions.PreferLogger = previousPreferLogger
	})

	_, _, err := exec.CmdWithContext(ctx, exec.Config{Print: true}, "echo", "non-muted output")
	require.NoError(t, err)
	_, _, err = exec.CmdWithContext(ctx, exec.Config{Print: false}, "echo", "muted output")
	require.NoError(t, err)

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "non-muted output", entries[0]["@message"])
}

func TestZarfStructuredOutputFieldsForwarded(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithZarfLogger(ctx)

	logger.From(ctx).Info("structured output",
		slog.String("stream", "stdout"),
		slog.String("password", "secret"),
		slog.String("request.token", "nested token"),
		slog.String("credentials.api_key", "nested api key"),
		slog.Any("resource_config", map[string]string{"secret": "never forward"}),
	)

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "structured output", entries[0]["@message"])
	require.Equal(t, "stdout", entries[0]["stream"])
	require.NotContains(t, entries[0], "resource_config")
	require.Equal(t, "secret", entries[0]["password"])
	require.Equal(t, "nested token", entries[0]["request.token"])
	require.Equal(t, "nested api key", entries[0]["credentials.api_key"])
}

func TestZarfCommandOutputOnFailurePreservesContext(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithPackageContext(ctx, "create", "podinfo", "demo")
	ctx = WithZarfLogger(ctx)
	preferLogger := zarfConfig.CommonOptions.PreferLogger
	zarfConfig.CommonOptions.PreferLogger = true
	t.Cleanup(func() { zarfConfig.CommonOptions.PreferLogger = preferLogger })

	command := "sh -c printf 'safe stdout\\n'; printf 'safe stderr\\n' >&2; exit 1"
	_, _, err := exec.CmdWithContext(ctx, exec.Config{Print: true}, "sh", "-c", "printf 'safe stdout\\n'; printf 'safe stderr\\n' >&2; exit 1")
	require.Error(t, err)
	logger.From(ctx).Warn("Zarf command failed", slog.String("cmd", command))

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		require.Equal(t, "create", entry[fieldOperation])
		require.Equal(t, "podinfo", entry[fieldPackage])
		require.Equal(t, "demo", entry[fieldNamespace])
	}
	assertLogMessage(t, entries, "safe stdout")
	assertLogMessage(t, entries, "safe stderr")
	for _, entry := range entries {
		if entry["@message"] == "Zarf command failed" {
			require.Equal(t, command, entry["cmd"])
			return
		}
	}
	t.Fatalf("log entry %q not found in %#v", "Zarf command failed", entries)
}

func TestZarfCommandOutputOnMutedFailureIsAbsent(t *testing.T) {
	var output bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &output)
	ctx = WithZarfLogger(ctx)
	preferLogger := zarfConfig.CommonOptions.PreferLogger
	zarfConfig.CommonOptions.PreferLogger = true
	t.Cleanup(func() { zarfConfig.CommonOptions.PreferLogger = preferLogger })

	_, _, err := exec.CmdWithContext(ctx, exec.Config{Print: false}, "sh", "-c", "printf 'muted stdout\\n'; printf 'muted stderr\\n' >&2; exit 1")
	require.Error(t, err)

	entries, err := tflogtest.MultilineJSONDecode(&output)
	require.NoError(t, err)
	require.Empty(t, entries)
}

// Diagnostic wrapping.

func TestWrapZarfErrorIncludesUnstructuredOutput(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	log := logger.From(ctx)
	log.Info("namespace already exists")
	log.Error("action failed")
	log.Info("structured sdk detail", slog.String("detail", "not Zarf output"))

	wrapped := WrapZarfError(ctx, errors.New("original failure"))

	require.ErrorContains(t, wrapped, "original failure")
	require.ErrorContains(t, wrapped, "captured Zarf output:")
	require.ErrorContains(t, wrapped, "namespace already exists")
	require.NotContains(t, wrapped.Error(), "structured sdk detail")
	require.NotContains(t, wrapped.Error(), "additional Zarf output omitted")
}

func TestWrapZarfErrorReturnsOriginalWithoutCapturedOutput(t *testing.T) {
	original := errors.New("original failure")
	emptyContext := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	require.ErrorIs(t, WrapZarfError(emptyContext, original), original)
	require.Equal(t, original, WrapZarfError(emptyContext, original))
}

func TestZarfCaptureOutputOmitsRecordsAfterRecordLimit(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	for i := 0; i < maxZarfOutputRecords+2; i++ {
		logger.From(ctx).Info(fmt.Sprintf("record-%02d", i))
	}

	wrapped := WrapZarfError(ctx, errors.New("original failure"))
	for i := 0; i < maxZarfOutputRecords; i++ {
		require.ErrorContains(t, wrapped, fmt.Sprintf("record-%02d", i))
	}
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 2 records, 18 bytes]")
	require.NotContains(t, wrapped.Error(), fmt.Sprintf("record-%02d", maxZarfOutputRecords))
}

func TestZarfCaptureOutputOmitsRecordsAfterByteLimit(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	first := strings.Repeat("a", maxZarfOutputBytes-4)
	logger.From(ctx).Info(first)
	logger.From(ctx).Info("second record")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))
	require.LessOrEqual(t, len(strings.SplitN(wrapped.Error(), "captured Zarf output:\n", 2)[1]), maxZarfOutputBytes+256)
	require.ErrorContains(t, wrapped, first)
	require.NotContains(t, wrapped.Error(), "second record")
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 13 bytes]")
}

func TestZarfCaptureOutputOmitsErrorAfterRecordLimit(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	for i := 0; i < maxZarfOutputRecords; i++ {
		logger.From(ctx).Info(fmt.Sprintf("info-%02d", i))
	}
	logger.From(ctx).Error("overflow error")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))

	require.NotContains(t, wrapped.Error(), "overflow error")
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 14 bytes]")
}

func TestZarfCaptureOutputOmitsOversizedRecordButAcceptsLaterOutput(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	logger.From(ctx).Info(strings.Repeat("a", maxZarfOutputBytes+1))
	logger.From(ctx).Info("small record")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))

	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 12289 bytes]")
	require.Contains(t, wrapped.Error(), "small record")
}

func assertLogMessage(t *testing.T, entries []map[string]interface{}, message string) {
	t.Helper()
	for _, entry := range entries {
		if entry["@message"] == message {
			return
		}
	}
	t.Fatalf("log entry %q not found in %#v", message, entries)
}
