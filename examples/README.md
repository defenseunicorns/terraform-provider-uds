# Examples

This directory contains examples used to generate the provider documentation. You can also run them manually with Terraform or OpenTofu.

The documentation generator reads examples from these conventional paths:

- `provider/provider.tf` for the provider index
- `data-sources/<full data source name>/data-source.tf` for a data source
- `resources/<full resource name>/resource.tf` for a resource

Other Terraform files can support runnable examples without appearing in the generated documentation. Run `uds run generate` from the repository root after changing a documented example or provider schema.
