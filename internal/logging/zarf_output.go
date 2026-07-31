// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logging

import (
	"fmt"
	"strings"
	"sync"
)

const (
	maxZarfOutputRecords = 48
	maxZarfOutputBytes   = 12 * 1024
)

type zarfOutputCollectorKey struct{}

type outputBuffer struct {
	lines      []string
	bytes      int
	maxRecords int
	maxBytes   int
}

// append adds message while retaining the head and tail when the buffer overflows.
func (b *outputBuffer) append(message string) (accepted bool, droppedRecords int, droppedBytes int) {
	messageBytes := len(message)
	if messageBytes > b.maxBytes {
		return false, 0, 0
	}
	separatorBytes := b.separatorBytes()
	droppedRecords, droppedBytes, ok := b.makeRoom(messageBytes, separatorBytes)
	if !ok {
		return false, droppedRecords, droppedBytes
	}

	b.lines = append(b.lines, message)
	b.bytes += separatorBytes + messageBytes
	return true, droppedRecords, droppedBytes
}

func (b *outputBuffer) separatorBytes() int {
	if len(b.lines) == 0 {
		return 0
	}
	return 1
}

func (b *outputBuffer) makeRoom(messageBytes, separatorBytes int) (droppedRecords int, droppedBytes int, ok bool) {
	for len(b.lines) >= b.maxRecords || b.bytes+separatorBytes+messageBytes > b.maxBytes {
		dropped, ok := b.dropTail()
		if !ok {
			return droppedRecords, droppedBytes, false
		}
		droppedRecords++
		droppedBytes += len(dropped)
	}
	return droppedRecords, droppedBytes, true
}

func (b *outputBuffer) dropTail() (string, bool) {
	tailStart := b.maxRecords / 2
	if tailStart > len(b.lines) {
		tailStart = len(b.lines)
	}
	if tailStart == len(b.lines) {
		return "", false
	}

	dropped := b.lines[tailStart]
	b.lines = append(b.lines[:tailStart], b.lines[tailStart+1:]...)
	b.bytes -= len(dropped) + 1
	return dropped, true
}

// zarfOutputCollector is shared by concurrent slog Handler calls for
// one operation, so mu protects captured output and omission state.
type zarfOutputCollector struct {
	mu sync.Mutex

	output outputBuffer

	omittedRecords int
	omittedBytes   int
}

func (c *zarfOutputCollector) add(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	accepted, droppedRecords, droppedBytes := c.output.append(message)
	c.omittedRecords += droppedRecords
	c.omittedBytes += droppedBytes
	if !accepted {
		c.omit(message)
	}
}

// Accounts for output that cannot fit within the record or byte limit.
func (c *zarfOutputCollector) omit(message string) {
	c.omittedRecords++
	c.omittedBytes += len(message)
}

func formatZarfOutput(lines []string, omittedRecords, omittedBytes int) string {
	if omittedRecords == 0 {
		return strings.Join(lines, "\n")
	}

	headCount := maxZarfOutputRecords / 2
	if headCount > len(lines) {
		headCount = len(lines)
	}
	head := strings.Join(lines[:headCount], "\n")
	tail := strings.Join(lines[headCount:], "\n")
	marker := fmt.Sprintf("[additional Zarf output omitted: %d records, %d bytes]", omittedRecords, omittedBytes)
	head, tail = fitZarfOutput(head, tail, maxZarfOutputBytes-len(marker))

	parts := make([]string, 0, 3)
	if head != "" {
		parts = append(parts, head)
	}
	parts = append(parts, marker)
	if tail != "" {
		parts = append(parts, tail)
	}
	return strings.Join(parts, "\n")
}

func fitZarfOutput(head, tail string, available int) (string, string) {
	if head != "" {
		available--
	}
	if tail != "" {
		available--
	}
	if tail == "" {
		return truncateZarfOutput(head, available, false), ""
	}

	headBudget := available / 2
	tailBudget := available - headBudget
	if len(head) < headBudget {
		tailBudget += headBudget - len(head)
	}
	if len(tail) < tailBudget {
		headBudget += tailBudget - len(tail)
	}
	return truncateZarfOutput(head, headBudget, false), truncateZarfOutput(tail, tailBudget, true)
}

func truncateZarfOutput(value string, maxBytes int, keepEnd bool) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}

	runes := []rune(value)
	if keepEnd {
		start := len(runes)
		for start > 0 && len(string(runes[start-1:])) <= maxBytes {
			start--
		}
		return string(runes[start:])
	}

	end := 0
	for end < len(runes) && len(string(runes[:end+1])) <= maxBytes {
		end++
	}
	return string(runes[:end])
}

// Creates a bounded buffer for captured Zarf output.
func newZarfOutputCollector() *zarfOutputCollector {
	return &zarfOutputCollector{
		output: outputBuffer{
			maxRecords: maxZarfOutputRecords,
			maxBytes:   maxZarfOutputBytes,
		},
	}
}
