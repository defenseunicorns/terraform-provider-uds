// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packager

import (
	"context"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// ZarfPackager defines the interface for interacting with Zarf packages
type ZarfPackager interface {
	Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) (packager.DeployResult, error)
	Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts packager.RemoveOptions) error
	LoadPackage(ctx context.Context, source string, opts packager.LoadOptions) (_ *layout.PackageLayout, err error)
}

// DefaultZarfPackager is the default implementation of ZarfPackager
type DefaultZarfPackager struct{}

// NewZarfPackager creates a new instance of DefaultZarfPackager
func NewZarfPackager() *DefaultZarfPackager {
	return &DefaultZarfPackager{}
}

// Deploy implements the ZarfPackager interface
func (z *DefaultZarfPackager) Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) (packager.DeployResult, error) {
	return packager.Deploy(ctx, pkgLayout, opts)
}

// Remove implements the ZarfPackager interface
func (z *DefaultZarfPackager) Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts packager.RemoveOptions) error {
	return packager.Remove(ctx, pkg, opts)
}

// Load implements the ZarfPackager interface
func (z *DefaultZarfPackager) LoadPackage(ctx context.Context, source string, opts packager.LoadOptions) (_ *layout.PackageLayout, err error) {
	return packager.LoadPackage(ctx, source, opts)
}
