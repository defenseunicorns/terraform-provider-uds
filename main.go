// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime/debug"

	"github.com/defenseunicorns/terraform-provider-uds/internal/provider"
	server "github.com/hashicorp/terraform-plugin-framework/providerserver"
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
)

var (
	// set by goreleaser at build time
	version = "dev"
)

func main() {

	// Check if the zarf command is being run
	if len(os.Args) > 1 && os.Args[1] == "zarf" {
		zarfCmd()
		return
	}

	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := server.ServeOpts{
		Address: "registry.terraform.io/defenseunicorns/uds",
		Debug:   debug,
	}

	err := server.Serve(context.Background(), provider.New(version), opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}

func zarfCmd() {
	// grab Zarf version to make Zarf library checks happy
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range buildInfo.Deps {
			if dep.Path == "github.com/zarf-dev/zarf" {
				zarfConfig.CLIVersion = dep.Version
			}
		}
	}

	os.Args = os.Args[1:] // grab 'zarf' and onward from the CLI args
	zarfCLI.Execute(context.TODO())
}
