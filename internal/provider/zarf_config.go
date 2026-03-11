// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"fmt"

	"gopkg.in/yaml.v2"
)

// zarfConfigFile represents the structure of a zarf-config.yaml file.
type zarfConfigFile struct {
	Package zarfConfigPackage `yaml:"package"`
}

type zarfConfigPackage struct {
	Deploy zarfConfigDeploy `yaml:"deploy"`
}

type zarfConfigDeploy struct {
	Set map[string]string `yaml:"set"`
}

// parseZarfConfigSetVariables parses a zarf-config.yaml string and extracts
// the package.deploy.set variables.
func parseZarfConfigSetVariables(content string) (map[string]string, error) {
	if content == "" {
		return nil, nil
	}

	var cfg zarfConfigFile
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse zarf config YAML: %w", err)
	}

	return cfg.Package.Deploy.Set, nil
}
