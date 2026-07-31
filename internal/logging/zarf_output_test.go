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

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/logger"
)

func TestOutputBufferEnforcesRecordAndByteLimits(t *testing.T) {
	tests := []struct {
		name       string
		maxRecords int
		maxBytes   int
		steps      []bufferStep
		wantLines  []string
		wantBytes  int
	}{
		{
			name:       "record limit",
			maxRecords: 2,
			maxBytes:   100,
			steps: []bufferStep{
				{message: "one", accepted: true},
				{message: "two", accepted: true},
				{message: "three", accepted: true, dropped: 1, droppedBytes: len("two")},
			},
			wantLines: []string{"one", "three"},
			wantBytes: len("one") + len("three") + 1,
		},
		{
			name:       "byte limit",
			maxRecords: 100,
			maxBytes:   5,
			steps: []bufferStep{
				{message: "one", accepted: true},
				{message: "three", accepted: false},
			},
			wantLines: []string{"one"},
			wantBytes: len("one"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := outputBuffer{maxRecords: tt.maxRecords, maxBytes: tt.maxBytes}
			for _, step := range tt.steps {
				accepted, dropped, droppedBytes := buffer.append(step.message)
				require.Equal(t, step.accepted, accepted, "message %q", step.message)
				require.Equal(t, step.dropped, dropped, "message %q", step.message)
				require.Equal(t, step.droppedBytes, droppedBytes, "message %q", step.message)
			}
			require.Equal(t, tt.wantLines, buffer.lines)
			require.Equal(t, tt.wantBytes, buffer.bytes)
		})
	}
}

type bufferStep struct {
	message      string
	accepted     bool
	dropped      int
	droppedBytes int
}

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
	require.ErrorContains(t, wrapped, "action failed")
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
	for i := 0; i < maxZarfOutputRecords/2; i++ {
		require.ErrorContains(t, wrapped, fmt.Sprintf("record-%02d", i))
	}
	for i := maxZarfOutputRecords/2 + 2; i < maxZarfOutputRecords+2; i++ {
		require.ErrorContains(t, wrapped, fmt.Sprintf("record-%02d", i))
	}
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 2 records, 18 bytes]")
	require.NotContains(t, wrapped.Error(), "record-24")
	require.NotContains(t, wrapped.Error(), "record-25")
	marker := strings.Index(wrapped.Error(), "[additional Zarf output omitted:")
	require.Greater(t, marker, strings.Index(wrapped.Error(), "record-23"))
	require.Less(t, marker, strings.Index(wrapped.Error(), "record-26"))
}

func TestZarfCaptureOutputOmitsRecordsAfterByteLimit(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	first := strings.Repeat("a", maxZarfOutputBytes-4)
	logger.From(ctx).Info(first)
	logger.From(ctx).Info("second record")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))
	captured := strings.SplitN(wrapped.Error(), "captured Zarf output:\n", 2)[1]
	require.LessOrEqual(t, len(captured), maxZarfOutputBytes)
	require.Contains(t, captured, first[:100])
	require.NotContains(t, wrapped.Error(), "second record")
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 13 bytes]")
	require.True(t, strings.HasSuffix(captured, "[additional Zarf output omitted: 1 records, 13 bytes]"))
}

func TestZarfCaptureOutputKeepsErrorAfterRecordLimit(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	for i := 0; i < maxZarfOutputRecords; i++ {
		logger.From(ctx).Info(fmt.Sprintf("info-%02d", i))
	}
	logger.From(ctx).Error("overflow error")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))

	require.Contains(t, wrapped.Error(), "overflow error")
	require.NotContains(t, wrapped.Error(), "info-24")
	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 7 bytes]")
	for i := 0; i < maxZarfOutputRecords/2; i++ {
		require.Contains(t, wrapped.Error(), fmt.Sprintf("info-%02d", i))
	}
	for i := maxZarfOutputRecords/2 + 1; i < maxZarfOutputRecords; i++ {
		require.Contains(t, wrapped.Error(), fmt.Sprintf("info-%02d", i))
	}
}

func TestZarfCaptureOutputOmitsOversizedRecordButAcceptsLaterOutput(t *testing.T) {
	ctx := WithZarfLogger(tflogtest.RootLogger(context.Background(), &bytes.Buffer{}))
	logger.From(ctx).Info(strings.Repeat("a", maxZarfOutputBytes+1))
	logger.From(ctx).Info("small record")

	wrapped := WrapZarfError(ctx, errors.New("original failure"))

	require.Contains(t, wrapped.Error(), "[additional Zarf output omitted: 1 records, 12289 bytes]")
	require.Contains(t, wrapped.Error(), "small record")
}
