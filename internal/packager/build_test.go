// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/logger"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/types"
)

func TestCreateForwardsOptionsAndWrapsOutput(t *testing.T) {
	sentinel := errors.New("build failed")
	opts := zPackager.CreateOptions{Architecture: "arm64", CachePath: "cache", RemoteOptions: types.RemoteOptions{PlainHTTP: true, InsecureSkipTLSVerify: true}}
	p := newTestZarfPackager(testZarfPackagerOptions{createPackage: func(ctx context.Context, source, output string, got zPackager.CreateOptions) (string, error) {
		require.Equal(t, "source", source)
		require.Equal(t, "output", output)
		require.Equal(t, opts, got)
		require.False(t, got.IsInteractive)
		require.Nil(t, got.SignBlobOptions)
		require.Empty(t, got.SetVariables)
		logger.From(ctx).Error("build action output")
		return "actual-archive.tar.zst", sentinel
	}})
	archive, err := p.Create(context.Background(), "source", "output", opts)
	require.Equal(t, "actual-archive.tar.zst", archive)
	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "build action output")
}

func TestCreateLocalPackagesWithoutStdout(t *testing.T) {
	// Capture process stdout only in this serial test; provider protocol output
	// must not contain library progress or build-action output.
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous; _ = writer.Close(); _ = reader.Close() })
	output := make(chan []byte, 1)
	go func() { data, _ := io.ReadAll(reader); output <- data }()
	for _, version := range []string{"v1alpha1", "v1beta1"} {
		t.Run(version, func(t *testing.T) {
			source := t.TempDir()
			definition := fmt.Sprintf("apiVersion: zarf.dev/%s\nkind: ZarfPackageConfig\nmetadata:\n  name: jit-test\n  version: 1.0.0\ncomponents:\n  - name: empty\n", version)
			require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte(definition), 0o600))
			dest := t.TempDir()
			p := NewPackager()
			archive, err := p.Create(context.Background(), source, dest, zPackager.CreateOptions{Architecture: "arm64", CachePath: t.TempDir()})
			require.NoError(t, err)
			require.Equal(t, dest, filepath.Dir(archive))
			built, err := p.LoadPackage(context.Background(), archive, zPackager.LoadOptions{VerificationStrategy: layout.VerifyNever})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, built.Cleanup()) })
			require.Equal(t, "arm64", built.Definition().Build.Architecture)
			require.False(t, built.IsSigned())
			archives, err := filepath.Glob(filepath.Join(source, "*.tar*"))
			require.NoError(t, err)
			require.Empty(t, archives)
		})
	}
	os.Stdout = previous
	require.NoError(t, writer.Close())
	require.Empty(t, string(<-output))
}

func TestBuildAndLoadOwnership(t *testing.T) {
	for _, failure := range []string{"", "build", "load", "cancel", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			source := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: owned\ncomponents:\n  - name: empty\n"), 0o600))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "timeout" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 10*time.Millisecond)
				defer stop()
			}
			var output, returned string
			cache := t.TempDir()
			loaded := false
			adapter := NewPackager()
			p := newTestZarfPackager(testZarfPackagerOptions{
				createPackage: func(ctx context.Context, dir, out string, opts zPackager.CreateOptions) (string, error) {
					require.Equal(t, source, dir)
					output = out
					require.NotEqual(t, source, output)
					require.False(t, opts.IsInteractive)
					require.Nil(t, opts.SignBlobOptions)
					require.Empty(t, opts.SetVariables)
					require.Equal(t, "amd64", opts.Architecture)
					require.Equal(t, cache, opts.CachePath)
					require.True(t, opts.PlainHTTP)
					require.True(t, opts.InsecureSkipTLSVerify)
					if failure == "cancel" {
						cancel()
						return "", ctx.Err()
					}
					if failure == "timeout" {
						<-ctx.Done()
						return "", ctx.Err()
					}
					if failure == "build" {
						return "", errors.New("build failed")
					}
					var err error
					returned, err = adapter.Create(ctx, dir, out, opts)
					return returned, err
				},
				loadPackage: func(ctx context.Context, archive string, opts zPackager.LoadOptions) (*layout.PackageLayout, error) {
					loaded = true
					require.Equal(t, returned, archive)
					require.Equal(t, "amd64", opts.Architecture)
					require.Equal(t, cache, opts.CachePath)
					require.True(t, opts.PlainHTTP)
					require.True(t, opts.InsecureSkipTLSVerify)
					require.Equal(t, layout.VerifyNever, opts.VerificationStrategy)
					if failure == "load" {
						return nil, errors.New("load failed")
					}
					return adapter.LoadPackage(ctx, archive, opts)
				},
			})
			cwd, err := os.Getwd()
			require.NoError(t, err)
			relative, err := filepath.Rel(cwd, source)
			require.NoError(t, err)
			input := BuildInput{SourceDir: relative, Architecture: "amd64", CachePath: cache, PlainHTTP: true, InsecureSkipTLSVerify: true}
			pkg, cleanup, err := BuildAndLoad(ctx, p, input)
			require.Equal(t, relative, input.SourceDir)
			if failure == "" {
				require.NoError(t, err)
				require.NotNil(t, pkg)
				require.FileExists(t, returned)
				require.NoError(t, cleanup())
			} else {
				require.Error(t, err)
				require.Nil(t, pkg)
				if failure == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				if failure == "timeout" {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			}
			require.Equal(t, failure == "" || failure == "load", loaded)
			require.NoDirExists(t, output)
			require.NoError(t, cleanup()) // Idempotent even after failure cleanup.
			archives, err := filepath.Glob(filepath.Join(source, "*.tar*"))
			require.NoError(t, err)
			require.Empty(t, archives)
		})
	}
}

func TestConcurrentBuildArchitectureIsolation(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte("kind: ZarfPackageConfig\nmetadata:\n  name: architecture\n  architecture: amd64\ncomponents:\n  - name: empty\n"), 0o600))
	p := NewPackager()
	archive, err := p.Create(context.Background(), source, t.TempDir(), zPackager.CreateOptions{Architecture: "amd64", CachePath: t.TempDir()})
	require.NoError(t, err)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, arch := range []string{"amd64", "arm64"} {
		wg.Add(1)
		cache := t.TempDir()
		go func() {
			defer wg.Done()
			<-start
			pkg, cleanup, err := BuildAndLoad(context.Background(), p, BuildInput{SourceDir: source, Architecture: arch, CachePath: cache})
			if err != nil {
				t.Error(err)
				return
			}
			defer func() {
				if err := cleanup(); err != nil {
					t.Error(err)
				}
			}()
			if got := pkg.Definition().Build.Architecture; got != arch {
				t.Errorf("built architecture = %s, want %s", got, arch)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		pkg, err := p.LoadPackage(context.Background(), archive, zPackager.LoadOptions{Architecture: "arm64", VerificationStrategy: layout.VerifyNever})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			if err := pkg.Cleanup(); err != nil {
				t.Error(err)
			}
		}()
		if got := pkg.Definition().Build.Architecture; got != "amd64" {
			t.Errorf("source archive architecture changed to %s", got)
		}
	}()
	close(start)
	wg.Wait()
}

func TestBuildActionCancellationAndRelativeDirectory(t *testing.T) {
	t.Skip("Upstream Zarf action cancellation waits on child-held output pipes and does not preserve context.DeadlineExceeded in the returned error; re-enable after https://github.com/zarf-dev/zarf/issues/1461 is resolved")

	source := t.TempDir()
	definition := `kind: ZarfPackageConfig
metadata:
  name: cancel-build
components:
  - name: action
    actions:
      onCreate:
        before:
          - cmd: test -f marker && sleep 30 && touch completed
`
	require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte(definition), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "marker"), nil, 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, err := BuildAndLoad(ctx, NewPackager(), BuildInput{SourceDir: source, Architecture: "amd64", CachePath: t.TempDir()})
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoFileExists(t, filepath.Join(source, "completed"), "cancelled action must not finish normally")
	require.Less(t, time.Since(started), 5*time.Second)
	archives, err := filepath.Glob(filepath.Join(source, "*.tar*"))
	require.NoError(t, err)
	require.Empty(t, archives)
}
