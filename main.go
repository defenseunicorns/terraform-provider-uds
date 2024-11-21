// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"context"
	"flag"
	"log"

	// Must be imported before Zarf to avoid init() ordering issues
	// todo(jeff-mccoy): this is kind of gross and should be revisited
	"github.com/defenseunicorns/terraform-provider-uds/internal/fixzarf"
	"github.com/defenseunicorns/terraform-provider-uds/internal/provider"
	server "github.com/hashicorp/terraform-plugin-framework/providerserver"
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
)

var (
	// set by goreleaser at build time
	version = "dev"
)

func main() {
	// Check if the zarf command is being run
	if fixzarf.IsZarf() {
		zarfCLI.Execute(context.TODO())
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
