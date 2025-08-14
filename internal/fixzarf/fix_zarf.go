// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package fixzarf provides utilities to fix zarf argument handling and initialization ordering.
package fixzarf

import (
	"os"
	"runtime/debug"

	zarfConfig "github.com/zarf-dev/zarf/src/config"
)

var isZarf bool

// We must use an init() function so this code runs before any other init() functions.
//
//nolint:gochecknoinits // Required for Zarf initialization order
func init() {
	// Check if the zarf command is being run
	if len(os.Args) > 1 && os.Args[1] == "zarf" {
		// If zarf is called, remove the zarf command from the args
		// to avoid zarf cmd race conditions with init() functions
		os.Args = os.Args[1:]

		// grab Zarf version to make Zarf library checks happy
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range buildInfo.Deps {
				if dep.Path == "github.com/zarf-dev/zarf" {
					zarfConfig.CLIVersion = dep.Version
				}
			}
		}

		isZarf = true
	}
}

// IsZarf returns true if the zarf command is being run.
func IsZarf() bool {
	return isZarf
}
