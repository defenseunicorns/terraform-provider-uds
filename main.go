// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"context"
	"flag"
	"log"

	"github.com/defenseunicorns/terraform-provider-uds/internal/provider"
	server "github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var (
	// set by goreleaser at build time
	version = "dev"
)

func main() {
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
