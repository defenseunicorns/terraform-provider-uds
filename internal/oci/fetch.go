// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package oci provides utilities for fetching artifacts from OCI registries.
package oci

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	dockerconfig "github.com/docker/cli/cli/config"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// FetchOptions configures how an OCI artifact is fetched.
type FetchOptions struct {
	PlainHTTP             bool
	InsecureSkipTLSVerify bool
	File                  string
	MediaType             string
}

// FetchArtifact fetches content from an OCI artifact at the given reference.
// It returns the layer content, its digest, and any error.
func FetchArtifact(ctx context.Context, reference string, opts FetchOptions) ([]byte, string, error) {
	rawRef := strings.TrimPrefix(reference, "oci://")
	ref, err := registry.ParseReference(rawRef)
	if err != nil {
		return nil, "", fmt.Errorf("invalid OCI reference %q: %w", reference, err)
	}

	repo, err := remote.NewRepository(rawRef)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create repository for %q: %w", reference, err)
	}

	repo.PlainHTTP = opts.PlainHTTP

	client := &auth.Client{
		Credential: dockerCredentialFunc(),
	}
	if opts.InsecureSkipTLSVerify {
		client.Client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // user-configured
			},
		}
	}
	repo.Client = client

	// Resolve reference to manifest descriptor
	manifestDesc, err := repo.Resolve(ctx, ref.Reference)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve %q: %w", reference, err)
	}

	// Fetch and parse manifest
	rc, err := repo.Fetch(ctx, manifestDesc)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch manifest for %q: %w", reference, err)
	}
	manifestBytes, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, "", fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, "", fmt.Errorf("failed to parse manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return nil, "", fmt.Errorf("artifact %q has no layers", reference)
	}

	// Find the target layer
	targetLayer, err := findTargetLayer(manifest.Layers, opts.File, opts.MediaType)
	if err != nil {
		return nil, "", err
	}

	// Fetch layer content
	rc, err = repo.Fetch(ctx, targetLayer)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch layer: %w", err)
	}
	content, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, "", fmt.Errorf("failed to read layer content: %w", err)
	}

	return content, string(targetLayer.Digest), nil
}

func findTargetLayer(layers []ocispec.Descriptor, file, mediaType string) (ocispec.Descriptor, error) {
	for _, layer := range layers {
		if file != "" {
			title, ok := layer.Annotations[ocispec.AnnotationTitle]
			if !ok || title != file {
				continue
			}
		}
		if mediaType != "" && layer.MediaType != mediaType {
			continue
		}
		return layer, nil
	}

	if file == "" && mediaType == "" {
		return layers[0], nil
	}

	if file != "" {
		return ocispec.Descriptor{}, fmt.Errorf("file %q not found in artifact layers", file)
	}
	return ocispec.Descriptor{}, fmt.Errorf("no layer with media type %q found", mediaType)
}

func dockerCredentialFunc() func(context.Context, string) (auth.Credential, error) {
	return func(_ context.Context, hostname string) (auth.Credential, error) {
		cfg, err := dockerconfig.Load("")
		if err != nil {
			return auth.EmptyCredential, nil
		}
		authConfig, err := cfg.GetAuthConfig(hostname)
		if err != nil {
			return auth.EmptyCredential, nil
		}
		cred := auth.Credential{
			Username:     authConfig.Username,
			Password:     authConfig.Password,
			RefreshToken: authConfig.IdentityToken,
		}
		if cred.Username == "" && cred.Password == "" && cred.RefreshToken == "" {
			return auth.EmptyCredential, nil
		}
		return cred, nil
	}
}
