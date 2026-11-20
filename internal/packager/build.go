// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zTypes "github.com/zarf-dev/zarf/src/types"
)

// BuildInput contains execution inputs only, not resource policy or state.
type BuildInput struct {
	SourceDir             string
	Architecture          string
	CachePath             string
	PlainHTTP             bool
	InsecureSkipTLSVerify bool
}

// BuildAndLoad acquires a single owned layout. The caller must invoke cleanup
// after success. Partial failures clean all acquired storage before returning.
func BuildAndLoad(ctx context.Context, p Packager, input BuildInput) (_ *layout.PackageLayout, cleanup func() error, err error) {
	source, err := filepath.Abs(input.SourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve build directory: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("access build directory: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, errors.New("build source_dir must be a directory")
	}
	output, err := os.MkdirTemp("", "uds-package-build-*")
	if err != nil {
		return nil, nil, fmt.Errorf("allocate build output: %w", err)
	}
	var pkg *layout.PackageLayout
	var once sync.Once
	var cleanupErr error
	cleanup = func() error {
		once.Do(func() {
			if pkg != nil {
				cleanupErr = pkg.Cleanup()
			}
			cleanupErr = errors.Join(cleanupErr, os.RemoveAll(output))
		})
		return cleanupErr
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, cleanup())
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, cleanup, err
	}
	// Construct only supported execution options here. Builds are unsigned and
	// noninteractive, with no create-time resource variables.
	opts := zPackager.CreateOptions{
		Architecture: input.Architecture,
		CachePath:    input.CachePath,
		RemoteOptions: zTypes.RemoteOptions{
			PlainHTTP:             input.PlainHTTP,
			InsecureSkipTLSVerify: input.InsecureSkipTLSVerify,
		},
	}
	archive, err := p.Create(ctx, source, output, opts)
	if err != nil {
		return nil, cleanup, fmt.Errorf("build package: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, cleanup, err
	}
	pkg, err = p.LoadPackage(ctx, archive, zPackager.LoadOptions{
		Architecture: opts.Architecture, CachePath: opts.CachePath,
		RemoteOptions: opts.RemoteOptions, Filter: filters.Empty(), VerificationStrategy: layout.VerifyNever,
	})
	if err != nil {
		return nil, cleanup, fmt.Errorf("load built package: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, cleanup, err
	}
	return pkg, cleanup, nil
}
