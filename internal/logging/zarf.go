// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/zarf-dev/zarf/src/pkg/logger"
)

const zarfSubsystem = "zarf"

type zarfAttr struct {
	groups []string
	attr   slog.Attr
}

type zarfHandler struct {
	ctx    context.Context
	attrs  []zarfAttr
	groups []string
}

// Zarf logging entry points.

// WithZarfLogger attaches a Zarf slog logger that forwards to Terraform logs.
func WithZarfLogger(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, zarfOutputCollectorKey{}, newZarfOutputCollector())
	return logger.WithContext(ctx, slog.New(NewZarfHandler(ctx)))
}

// WrapZarfError adds captured Zarf output to an error when output was collected.
func WrapZarfError(ctx context.Context, err error) error {
	collector, _ := ctx.Value(zarfOutputCollectorKey{}).(*zarfOutputCollector)
	if collector == nil {
		return err
	}

	collector.mu.Lock()
	lines := append([]string(nil), collector.output.lines...)
	omittedRecords := collector.omittedRecords
	omittedBytes := collector.omittedBytes
	collector.mu.Unlock()
	if len(lines) == 0 && omittedRecords == 0 {
		return err
	}

	output := "captured Zarf output:\n" + formatZarfOutput(lines, omittedRecords, omittedBytes)
	return fmt.Errorf("%w\n\n%s", err, output)
}

// NewZarfHandler returns a slog handler backed by the Zarf Terraform subsystem.
func NewZarfHandler(ctx context.Context) slog.Handler {
	ctx = tflog.NewSubsystem(ctx, zarfSubsystem, tflog.WithRootFields())
	return &zarfHandler{ctx: ctx}
}

// Zarf slog handler implementation.

// Keeps all records available for host-side severity filtering.
func (h *zarfHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Identifies levels eligible for capture as unstructured Zarf output.
func isZarfOutputLevel(level slog.Level) bool {
	return level >= slog.LevelInfo && level < slog.LevelWarn || level >= slog.LevelError
}

// Forwards records to Terraform logs and captures eligible unstructured output.
func (h *zarfHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make(map[string]interface{}, len(h.attrs))
	for _, attr := range h.attrs {
		addZarfField(fields, attr.attr, attr.groups)
	}
	hasRecordAttrs := false
	record.Attrs(func(attr slog.Attr) bool {
		hasRecordAttrs = true
		addZarfField(fields, attr, h.groups)
		return true
	})

	switch {
	case record.Level < slog.LevelDebug:
		tflog.SubsystemTrace(h.ctx, zarfSubsystem, record.Message, fields)
	case record.Level < slog.LevelInfo:
		tflog.SubsystemDebug(h.ctx, zarfSubsystem, record.Message, fields)
	case record.Level < slog.LevelWarn:
		tflog.SubsystemInfo(h.ctx, zarfSubsystem, record.Message, fields)
	case record.Level < slog.LevelError:
		tflog.SubsystemWarn(h.ctx, zarfSubsystem, record.Message, fields)
	default:
		tflog.SubsystemError(h.ctx, zarfSubsystem, record.Message, fields)
	}

	if h.shouldCaptureZarfOutput(record, hasRecordAttrs) {
		if message := strings.TrimSpace(record.Message); message != "" {
			if collector, _ := h.ctx.Value(zarfOutputCollectorKey{}).(*zarfOutputCollector); collector != nil {
				collector.add(message)
			}
		}
	}
	return nil
}

// Returns a handler carrying attributes inherited from its parent.
func (h *zarfHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	all := make([]zarfAttr, 0, len(h.attrs)+len(attrs))
	all = append(all, h.attrs...)
	for _, attr := range attrs {
		all = append(all, zarfAttr{groups: h.groups, attr: attr})
	}
	return &zarfHandler{ctx: h.ctx, attrs: all, groups: h.groups}
}

// Returns a handler carrying the current nested attribute group.
func (h *zarfHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)
	return &zarfHandler{ctx: h.ctx, attrs: h.attrs, groups: groups}
}

func (h *zarfHandler) shouldCaptureZarfOutput(record slog.Record, hasRecordAttrs bool) bool {
	return len(h.attrs) == 0 && !hasRecordAttrs && isZarfOutputLevel(record.Level)
}

// Structured field conversion.

// Flattens nested groups into dotted keys and forwards supported scalar values.
func addZarfField(fields map[string]interface{}, attr slog.Attr, groups []string) {
	if attr.Key == "" {
		return
	}

	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		group := append(groups, attr.Key)
		for _, nested := range value.Group() {
			addZarfField(fields, nested, group)
		}
		return
	}
	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string(nil), groups...), key), ".")
	}
	switch value.Kind() {
	case slog.KindString:
		fields[key] = value.String()
	case slog.KindBool:
		fields[key] = value.Bool()
	case slog.KindInt64:
		fields[key] = value.Int64()
	case slog.KindUint64:
		fields[key] = value.Uint64()
	case slog.KindFloat64:
		fields[key] = value.Float64()
	case slog.KindDuration:
		fields[key] = value.Duration().String()
	case slog.KindTime:
		fields[key] = value.Time()
	}
}
