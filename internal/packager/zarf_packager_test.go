// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packager

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	zarfAPI "github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	"helm.sh/helm/v4/pkg/kube"
)

func TestZarfPackagerDeployDelegatesWithLoggerContextAndReturnsResult(t *testing.T) {
	pkgLayout := &layout.PackageLayout{}
	options := zPackager.DeployOptions{}
	want := zPackager.DeployResult{DeployedComponents: []zarfState.DeployedComponent{{Name: "sentinel"}}}
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		deployPackage: func(ctx context.Context, gotLayout *layout.PackageLayout, gotOptions zPackager.DeployOptions) (zPackager.DeployResult, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			require.Same(t, pkgLayout, gotLayout)
			require.Equal(t, options, gotOptions)
			return want, nil
		},
	})

	result, err := p.Deploy(context.Background(), pkgLayout, options)
	require.NoError(t, err)
	require.Equal(t, want, result)
	require.True(t, called)
}

func TestZarfPackagerDeployWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("deploy failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		deployPackage: func(ctx context.Context, _ *layout.PackageLayout, _ zPackager.DeployOptions) (zPackager.DeployResult, error) {
			logger.From(ctx).Error("deploy Zarf command output")
			return zPackager.DeployResult{}, sentinel
		},
	})

	_, err := p.Deploy(context.Background(), nil, zPackager.DeployOptions{})

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "captured Zarf output:")
	require.ErrorContains(t, err, "deploy Zarf command output")
}

func TestZarfPackagerRemoveDelegatesArgumentsWithLoggerContext(t *testing.T) {
	pkg := zarfAPI.NewPackageDefinitionFromV1alpha1(v1alpha1.ZarfPackage{Metadata: v1alpha1.ZarfMetadata{Name: "sentinel"}})
	options := zPackager.RemoveOptions{}
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		removePackage: func(ctx context.Context, gotPackage zarfAPI.PackageDefinition, gotOptions zPackager.RemoveOptions) error {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			require.Equal(t, pkg, gotPackage)
			require.Equal(t, options, gotOptions)
			return nil
		},
	})

	err := p.Remove(context.Background(), pkg, options)
	require.NoError(t, err)
	require.True(t, called)
}

func TestZarfPackagerRemoveWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("remove failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		removePackage: func(ctx context.Context, _ zarfAPI.PackageDefinition, _ zPackager.RemoveOptions) error {
			logger.From(ctx).Error("remove Zarf command output")
			return sentinel
		},
	})

	err := p.Remove(context.Background(), zarfAPI.NewPackageDefinitionFromV1alpha1(v1alpha1.ZarfPackage{}), zPackager.RemoveOptions{})

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "captured Zarf output:")
	require.ErrorContains(t, err, "remove Zarf command output")
}

func TestZarfPackagerLoadPackageDelegatesWithLoggerContextAndReturnsLayout(t *testing.T) {
	const source = "sentinel-source"
	options := zPackager.LoadOptions{}
	want := &layout.PackageLayout{}
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		loadPackage: func(ctx context.Context, gotSource string, gotOptions zPackager.LoadOptions) (*layout.PackageLayout, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			require.Equal(t, source, gotSource)
			require.Equal(t, options, gotOptions)
			return want, nil
		},
	})

	result, err := p.LoadPackage(context.Background(), source, options)
	require.NoError(t, err)
	require.Same(t, want, result)
	require.True(t, called)
}

func TestZarfPackagerLoadPackageWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("load failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		loadPackage: func(ctx context.Context, _ string, _ zPackager.LoadOptions) (*layout.PackageLayout, error) {
			logger.From(ctx).Error("load Zarf command output")
			return nil, sentinel
		},
	})

	_, err := p.LoadPackage(context.Background(), "source", zPackager.LoadOptions{})

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "captured Zarf output:")
	require.ErrorContains(t, err, "load Zarf command output")
}

func TestZarfPackagerGetPackageDelegatesWithLoggerContextAndReturnsPackage(t *testing.T) {
	const source = "sentinel-source"
	const namespace = "sentinel-namespace"
	options := zPackager.LoadOptions{}
	clusterValue := (*cluster.Cluster)(nil)
	want := zarfAPI.NewPackageDefinitionFromV1alpha1(v1alpha1.ZarfPackage{Metadata: v1alpha1.ZarfMetadata{Name: "sentinel"}})
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		getPackage: func(ctx context.Context, gotCluster *cluster.Cluster, gotSource string, gotNamespace string, gotOptions zPackager.LoadOptions) (zarfAPI.PackageDefinition, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			require.Equal(t, clusterValue, gotCluster)
			require.Equal(t, source, gotSource)
			require.Equal(t, namespace, gotNamespace)
			require.Equal(t, options, gotOptions)
			return want, nil
		},
	})

	result, err := p.GetPackageFromSourceOrCluster(context.Background(), clusterValue, source, namespace, options)
	require.NoError(t, err)
	require.Equal(t, want, result)
	require.True(t, called)
}

func TestNewPackagerInitializesDelegates(t *testing.T) {
	p := NewPackager().(*zarfPackager)
	require.NotNil(t, p.loadPackage)
	require.NotNil(t, p.deployPackage)
	require.NotNil(t, p.getPackage)
	require.NotNil(t, p.removePackage)
}

func TestZarfPackagerGetPackageWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("get package failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		getPackage: func(ctx context.Context, _ *cluster.Cluster, _ string, _ string, _ zPackager.LoadOptions) (zarfAPI.PackageDefinition, error) {
			logger.From(ctx).Error("get package Zarf command output")
			return zarfAPI.NewPackageDefinitionFromV1alpha1(v1alpha1.ZarfPackage{}), sentinel
		},
	})

	_, err := p.GetPackageFromSourceOrCluster(context.Background(), nil, "source", "", zPackager.LoadOptions{})

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "captured Zarf output:")
	require.ErrorContains(t, err, "get package Zarf command output")
}

func TestZarfPackagerSetsPreferLogger(t *testing.T) {
	p := &zarfPackager{}
	p.ensureZarfConfigured()
	p.ensureZarfConfigured()

	require.True(t, zarfConfig.CommonOptions.PreferLogger)
}

func TestZarfPackagerConfiguresConcurrently(t *testing.T) {
	const packagerCount = 100
	var wg sync.WaitGroup
	wg.Add(packagerCount)

	for range packagerCount {
		go func() {
			defer wg.Done()
			p := &zarfPackager{}
			p.ensureZarfConfigured()
			p.ensureZarfConfigured()
		}()
	}

	wg.Wait()
	require.Equal(t, cluster.FieldManagerName, kube.ManagedFieldsManager)
	require.Equal(t, "zarf", zarfConfig.ActionsCommandZarfPrefix)
	require.True(t, zarfConfig.CommonOptions.PreferLogger)
}

type testZarfPackagerOptions struct {
	loadPackage   loadPackageFunc
	deployPackage deployFunc
	getPackage    getPackageFunc
	removePackage removeFunc
}

func newTestZarfPackager(options testZarfPackagerOptions) *zarfPackager {
	return &zarfPackager{
		loadPackage:   options.loadPackage,
		deployPackage: options.deployPackage,
		getPackage:    options.getPackage,
		removePackage: options.removePackage,
	}
}
