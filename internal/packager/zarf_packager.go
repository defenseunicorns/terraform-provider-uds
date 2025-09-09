// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package packager provides interfaces and implementations for Zarf package operations.
package packager

import (
	"context"
	"runtime"
	"strings"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// Packager provides operations for managing Zarf packages.
type Packager interface {
	Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts zPackager.DeployOptions) (zPackager.DeployResult, error)
	Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts zPackager.RemoveOptions) error
	LoadPackage(ctx context.Context, source string, opts zPackager.LoadOptions) (_ *layout.PackageLayout, err error)
	GetPackageFromSourceOrCluster(ctx context.Context, cluster *cluster.Cluster, src string, namespaceOverride string, opts zPackager.LoadOptions) (_ v1alpha1.ZarfPackage, err error)
}

type zarfPackager struct{}

// NewPackager creates a new instance of the Packager interface.
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

func (z *zarfPackager) GetPackageFromSourceOrCluster(ctx context.Context, cluster *cluster.Cluster, src string, namespaceOverride string, opts zPackager.LoadOptions) (_ v1alpha1.ZarfPackage, err error) {
	return zPackager.GetPackageFromSourceOrCluster(ctx, cluster, src, namespaceOverride, opts)
}

// PackageComponentFilter provides filtering strategies for Zarf package components.
type PackageComponentFilter interface {
	ForRemove(optionalComponentsp []string) filters.ComponentFilterStrategy
	ForDeploy(optionalComponents []string) filters.ComponentFilterStrategy
}

type zarfPackageComponentFilter struct{}

func (z *zarfPackageComponentFilter) ForRemove(optionalComponents []string) filters.ComponentFilterStrategy {
	if len(optionalComponents) > 0 {
		return filters.Combine(
			filters.ByLocalOS(runtime.GOOS),
			filters.BySelectState(strings.Join(optionalComponents, ",")),
		)
	}

	return filters.Combine(
		filters.ByLocalOS(runtime.GOOS),
	)
}

func (z *zarfPackageComponentFilter) ForDeploy(optionalComponents []string) filters.ComponentFilterStrategy {
	return filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
}

// NewPackageComponentFilter creates a new instance of the PackageComponentFilter interface.
func NewPackageComponentFilter() PackageComponentFilter {
	return &zarfPackageComponentFilter{}
}
