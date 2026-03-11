// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/defenseunicorns/pkg/helpers/v2"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

const testZarfConfigContent = `package:
  deploy:
    set:
      REGISTRY_URL: "registry.example.com"
      REGISTRY_USERNAME: "admin"
`

func TestAccOCIArtifactDataSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	port, err := helpers.GetAvailablePort()
	require.NoError(t, err)
	registryAddr := setupInMemoryRegistry(ctx, t, port)

	// Push a test artifact to the in-memory registry
	ref := fmt.Sprintf("%s/test/zarf-config:v1", registryAddr)
	pushTestArtifact(ctx, t, ref, "zarf-config.yaml", []byte(testZarfConfigContent))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "uds" {
  insecure_force_http = true
}

data "uds_oci_artifact" "zarf_config" {
  reference = "oci://%s"
  file      = "zarf-config.yaml"
}
`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uds_oci_artifact.zarf_config", "content", testZarfConfigContent),
					resource.TestCheckResourceAttrSet("data.uds_oci_artifact.zarf_config", "digest"),
					resource.TestCheckResourceAttr("data.uds_oci_artifact.zarf_config", "id", "oci://"+ref),
				),
			},
		},
	})
}

func TestAccOCIArtifactDataSource_FirstLayer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	port, err := helpers.GetAvailablePort()
	require.NoError(t, err)
	registryAddr := setupInMemoryRegistry(ctx, t, port)

	// Push a single-layer artifact without specifying file filter
	ref := fmt.Sprintf("%s/test/simple:v1", registryAddr)
	pushTestArtifact(ctx, t, ref, "config.yaml", []byte("key: value\n"))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "uds" {
  insecure_force_http = true
}

data "uds_oci_artifact" "simple" {
  reference = "oci://%s"
}
`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uds_oci_artifact.simple", "content", "key: value\n"),
				),
			},
		},
	})
}

// pushTestArtifact pushes a single-file OCI artifact to the given reference using ORAS.
func pushTestArtifact(ctx context.Context, t *testing.T, ref string, fileName string, fileContent []byte) {
	t.Helper()

	repo, err := remote.NewRepository(ref)
	require.NoError(t, err)
	repo.PlainHTTP = true

	// Push the file as a blob layer
	layerDesc := content.NewDescriptorFromBytes("application/octet-stream", fileContent)
	layerDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: fileName,
	}
	require.NoError(t, repo.Push(ctx, layerDesc, bytes.NewReader(fileContent)))

	// Push an empty config blob
	configContent := []byte("{}")
	configDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeEmptyJSON, configContent)
	require.NoError(t, repo.Push(ctx, configDesc, bytes.NewReader(configContent)))

	// Build and push the manifest
	manifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
	}
	manifest.SchemaVersion = 2
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)

	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	require.NoError(t, repo.PushReference(ctx, manifestDesc, bytes.NewReader(manifestBytes), ref))
}
