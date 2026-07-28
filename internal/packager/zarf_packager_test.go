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
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"helm.sh/helm/v4/pkg/kube"
)

func TestZarfPackagerDeployUsesLoggerContext(t *testing.T) {
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		deployPackage: func(ctx context.Context, _ *layout.PackageLayout, _ zPackager.DeployOptions) (zPackager.DeployResult, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			return zPackager.DeployResult{}, nil
		},
	})

	_, err := p.Deploy(context.Background(), nil, zPackager.DeployOptions{})
	require.NoError(t, err)
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

func TestZarfPackagerRemoveUsesLoggerContext(t *testing.T) {
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		removePackage: func(ctx context.Context, _ v1alpha1.ZarfPackage, _ zPackager.RemoveOptions) error {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			return nil
		},
	})

	err := p.Remove(context.Background(), v1alpha1.ZarfPackage{}, zPackager.RemoveOptions{})
	require.NoError(t, err)
	require.True(t, called)
}

func TestZarfPackagerRemoveWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("remove failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		removePackage: func(ctx context.Context, _ v1alpha1.ZarfPackage, _ zPackager.RemoveOptions) error {
			logger.From(ctx).Error("remove Zarf command output")
			return sentinel
		},
	})

	err := p.Remove(context.Background(), v1alpha1.ZarfPackage{}, zPackager.RemoveOptions{})

	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "captured Zarf output:")
	require.ErrorContains(t, err, "remove Zarf command output")
}

func TestZarfPackagerLoadPackageUsesLoggerContext(t *testing.T) {
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		loadPackage: func(ctx context.Context, _ string, _ zPackager.LoadOptions) (*layout.PackageLayout, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			return nil, nil
		},
	})

	_, err := p.LoadPackage(context.Background(), "source", zPackager.LoadOptions{})
	require.NoError(t, err)
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

func TestZarfPackagerGetPackageUsesLoggerContext(t *testing.T) {
	called := false
	p := newTestZarfPackager(testZarfPackagerOptions{
		getPackage: func(ctx context.Context, _ *cluster.Cluster, _ string, _ string, _ zPackager.LoadOptions) (v1alpha1.ZarfPackage, error) {
			called = true
			require.True(t, logger.From(ctx).Enabled(ctx, slog.LevelInfo))
			return v1alpha1.ZarfPackage{}, nil
		},
	})

	_, err := p.GetPackageFromSourceOrCluster(context.Background(), nil, "source", "", zPackager.LoadOptions{})
	require.NoError(t, err)
	require.True(t, called)
}

func TestZarfPackagerGetPackageWrapsCapturedOutputOnError(t *testing.T) {
	sentinel := errors.New("get package failed")
	p := newTestZarfPackager(testZarfPackagerOptions{
		getPackage: func(ctx context.Context, _ *cluster.Cluster, _ string, _ string, _ zPackager.LoadOptions) (v1alpha1.ZarfPackage, error) {
			logger.From(ctx).Error("get package Zarf command output")
			return v1alpha1.ZarfPackage{}, sentinel
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
