// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main filters SPDX headers from rendered documentation code blocks.
package main

import (
	"os"
	"path/filepath"
	"regexp"
)

var spdxHeader = regexp.MustCompile("(?m)(^```[^\\n]*\\n)# Copyright [^\\n]* Defense Unicorns\\n# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial\\n\\n")

func main() {
	err := filepath.Walk("docs", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := spdxHeader.ReplaceAll(contents, []byte("${1}"))
		if string(updated) == string(contents) {
			return nil
		}

		return os.WriteFile(path, updated, info.Mode())
	})
	if err != nil {
		panic(err)
	}
}
