// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package packager provides interfaces and implementations for Zarf package operations.
package packager

import (
	"context"
	"runtime"
	"strings"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"helm.sh/helm/v4/pkg/kube"
)

// Packager provides operations for managing Zarf packages.
type Packager interface {
	Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts zPackager.DeployOptions) (zPackager.DeployResult, error)
	Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts zPackager.RemoveOptions) error
	LoadPackage(ctx context.Context, source string, opts zPackager.LoadOptions) (_ *layout.PackageLayout, err error)
	GetPackageFromSourceOrCluster(ctx context.Context, cluster *cluster.Cluster, src string, namespaceOverride string, opts zPackager.LoadOptions) (_ v1alpha1.ZarfPackage, err error)
}

type zarfPackager struct {
	zarfConfigured bool
}

// NewPackager creates a new instance of the Packager interface.
func NewPackager() Packager {
	return &zarfPackager{}
}

func (p *zarfPackager) Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts zPackager.DeployOptions) (zPackager.DeployResult, error) {
	p.ensureZarfConfigured()
	return zPackager.Deploy(ctx, pkgLayout, opts)
}

func (p *zarfPackager) Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts zPackager.RemoveOptions) error {
	p.ensureZarfConfigured()
	return zPackager.Remove(ctx, pkg, opts)
}

func (p *zarfPackager) LoadPackage(ctx context.Context, source string, opts zPackager.LoadOptions) (_ *layout.PackageLayout, err error) {
	p.ensureZarfConfigured()
	return zPackager.LoadPackage(ctx, source, opts)
}

func (p *zarfPackager) GetPackageFromSourceOrCluster(ctx context.Context, cluster *cluster.Cluster, src string, namespaceOverride string, opts zPackager.LoadOptions) (_ v1alpha1.ZarfPackage, err error) {
	p.ensureZarfConfigured()
	return zPackager.GetPackageFromSourceOrCluster(ctx, cluster, src, namespaceOverride, opts)
}

func (p *zarfPackager) ensureZarfConfigured() {
	if p.zarfConfigured {
		return
	}

	// Set the Helm field manager name to match Zarf's so that resources deployed via this provider,
	// uds, and/or Zarf are interchangeable without requiring force_helm_ssa_conflicts set to true.
	kube.ManagedFieldsManager = cluster.FieldManagerName

	// Set the prefix for `./zarf` actions since we have to vendor zarf. This only needs to be done once per instance.
	zConfig.ActionsCommandZarfPrefix = "zarf"

	p.zarfConfigured = true
}

// PackageComponentFilter provides filtering strategies for Zarf package components.
type PackageComponentFilter interface {
	ForRemove(optionalComponents []string) filters.ComponentFilterStrategy
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
	if len(optionalComponents) == 0 {
		return &requiredOnlyComponentFilter{}
	}
	return filters.Combine(
		filters.ForDeploy(strings.Join(optionalComponents, ","), false),
	)
}

// requiredOnlyComponentFilter selects only required package components, excluding optional ones (including Zarf defaults).
type requiredOnlyComponentFilter struct{}

func (f *requiredOnlyComponentFilter) Apply(pkg v1alpha1.ZarfPackage) ([]v1alpha1.ZarfComponent, error) {
	required := make([]v1alpha1.ZarfComponent, 0)
	for _, c := range pkg.Components {
		if c.IsRequired() {
			required = append(required, c)
		}
	}
	return required, nil
}

// NewPackageComponentFilter creates a new instance of the PackageComponentFilter interface.
func NewPackageComponentFilter() PackageComponentFilter {
	return &zarfPackageComponentFilter{}
}
