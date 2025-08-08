// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packager

import (
	"context"
	"runtime"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

type Packager interface {
	Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts zPackager.DeployOptions) (zPackager.DeployResult, error)
	Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts zPackager.RemoveOptions) error
	LoadPackage(ctx context.Context, source string, opts zPackager.LoadOptions) (_ *layout.PackageLayout, err error)
}

type zarfPackager struct{}

func NewPackager() Packager {
	return &zarfPackager{}
}

func (z *zarfPackager) Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts zPackager.DeployOptions) (zPackager.DeployResult, error) {
	return zPackager.Deploy(ctx, pkgLayout, opts)
}

func (z *zarfPackager) Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts zPackager.RemoveOptions) error {
	return zPackager.Remove(ctx, pkg, opts)
}

func (z *zarfPackager) LoadPackage(ctx context.Context, source string, opts zPackager.LoadOptions) (_ *layout.PackageLayout, err error) {
	return zPackager.LoadPackage(ctx, source, opts)
}

type PackageComponentFilter interface {
	ByLocalOS() filters.ComponentFilterStrategy
	ForDeploy(optionalComponents string) filters.ComponentFilterStrategy
}

type zarfPackageComponentFilter struct{}

func (z *zarfPackageComponentFilter) ByLocalOS() filters.ComponentFilterStrategy {
	return filters.Combine(
		filters.ByLocalOS(runtime.GOOS),
	)
}

func (z *zarfPackageComponentFilter) ForDeploy(optionalComponents string) filters.ComponentFilterStrategy {
	return filters.Combine(
		//filters.ByLocalOS(runtime.GOOS),
		filters.ForDeploy(optionalComponents, false),
	)
}

func NewPackageComponentFilter() PackageComponentFilter {
	return &zarfPackageComponentFilter{}
}
