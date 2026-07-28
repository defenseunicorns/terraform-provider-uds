// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package logging integrates provider and Zarf logging with Terraform logs.
package logging

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	fieldOperation = "uds.operation"
	fieldPackage   = "uds.package"
	fieldNamespace = "uds.namespace"
	fieldComponent = "uds.component"
)

// WithPackageContext adds package operation fields to a logging context.
func WithPackageContext(ctx context.Context, operation, packageName, namespace string) context.Context {
	if operation != "" {
		ctx = tflog.SetField(ctx, fieldOperation, operation)
	}
	if packageName != "" {
		ctx = tflog.SetField(ctx, fieldPackage, packageName)
	}
	if namespace != "" {
		ctx = tflog.SetField(ctx, fieldNamespace, namespace)
	}
	return ctx
}

// PackageStarted logs the beginning of package deployment.
func PackageStarted(ctx context.Context, architecture string, optionalComponents []string) {
	fields := map[string]interface{}{"architecture": architecture}
	if len(optionalComponents) > 0 {
		fields["optional_components"] = strings.Join(optionalComponents, ",")
	}
	tflog.Info(ctx, "package started", fields)
}

// ComponentStarted logs the beginning of component deployment or removal.
func ComponentStarted(ctx context.Context, component string) {
	tflog.Info(ctx, "component started", map[string]interface{}{fieldComponent: component})
}

// PackageCompleted logs successful package processing and its duration.
func PackageCompleted(ctx context.Context, duration time.Duration) {
	tflog.Info(ctx, "package completed", map[string]interface{}{"duration": duration.String()})
}

// PackageFailed logs a package processing failure and its component context.
func PackageFailed(ctx context.Context, component string, err error) {
	tflog.Error(ctx, "package failed", map[string]interface{}{
		fieldComponent: component,
		"error":        err.Error(),
	})
}

// ZarfConnectCommand logs the executable command for a deployed Zarf connect string.
func ZarfConnectCommand(ctx context.Context, name, description string) {
	command := "zarf connect " + name
	tflog.Info(ctx, command, map[string]interface{}{
		"command":     command,
		"description": description,
	})
}
