// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main provides the entrypoint for the UDS Terraform provider.
package main

import (
	"context"
	"flag"
	"log"

	// Must be imported before Zarf to avoid init() ordering issues
	// todo(jeff-mccoy): this is kind of gross and should be revisited
	server "github.com/hashicorp/terraform-plugin-framework/providerserver"
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
	zarfConfig "github.com/zarf-dev/zarf/src/config"

	"github.com/defenseunicorns/terraform-provider-uds/internal/fixzarf"
	"github.com/defenseunicorns/terraform-provider-uds/internal/provider"
)

//go:generate tofu fmt -recursive examples/
//go:generate go tool tfplugindocs generate -provider-name uds

// set by goreleaser at build time
var version = "dev"

func main() {
	ctx := context.Background()

	// Check if the zarf command is being run
	if fixzarf.IsZarf() {
		zarfConfig.CommonOptions.Confirm = true
		zarfConfig.ActionsCommandZarfPrefix = "zarf"
		zarfCLI.Execute(ctx)
		return
	}

	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := server.ServeOpts{
		Address: "registry.terraform.io/defenseunicorns/uds",
		Debug:   debug,
	}

	err := server.Serve(ctx, provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
