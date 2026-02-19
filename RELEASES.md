# UDS Tofu Provider Release Process

This document provides guidance on how to create releases of the UDS Tofu provider, address release issues, and other helpful tips.

This project uses [Release Please](https://github.com/googleapis/release-please) to automate release management and [GoReleaser](https://github.com/goreleaser/goreleaser-action) to build and publish release artifacts.

## How Releases Work

### Automated Releases (Release Please)

Release Please automatically:

1. Monitors commits to `main` and creates or updates a release PR with changelog entries
1. When the release PR is merged, it creates and pushes a version tag (e.g., `v0.1.0`)
1. The tag push triggers the existing release workflow ([`.github/workflows/release.yml`](.github/workflows/release.yml))

> **Note:** The UDS Tofu Provider is currently in pre-1.0 (`v0.x.y`) development.
> While the provider remains in `v0.x`, breaking changes increment the **minor** version.
> A `1.0.0` release will be created intentionally once the provider API is considered stable.

The release PR accumulates changes based on [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` → minor version bump
- `fix:`, `perf:`, and `refactor:` → patch version bump
- Any commit with `!` or a `BREAKING CHANGE:` footer → minor version bump (while in v0.x)
  - Example: `refactor!: remove deprecated attribute`

## Release Cadence

The UDS Tofu provider targets a release at least every two weeks. Final timing is determined by the team once release goals are met, with an ideal cadence of 1-2 weeks between releases.

## Release Checklist

### Standard Releases (via Release Please)

- [ ] Review and merge the open Release Please PR
- [ ] Confirm the version tag was created and pushed (this triggers the release workflow)
- [ ] Review the GitHub release:
  - [ ] Add a high-level summary of changes
  - [ ] Document any required upgrade steps or breaking changes
- [ ] Verify the GoReleaser workflow completed successfully and review release assets

### Manual Releases (if required)

Manual releases may be necessary for exceptional cases.

- [ ] Review open [Pull Requests](https://github.com/defenseunicorns/terraform-provider-uds/pulls)
- [ ] Create and push the version tag

  ```bash
  git tag -sa vX.Y.Z -m "vX.Y.Z"
  git push origin vX.Y.Z
  ```

- [ ] Update `.release-please-manifest.json` to reflect the new version
- [ ] Review the GitHub release:
  - [ ] Add a high-level summary of changes
  - [ ] Document any required upgrade steps or breaking changes
- [ ] Verify the GoReleaser workflow completed successfully and review release assets

## Handling Release Issues

### A Release is Broken and Should Not Be Used

Do not delete a broken release. Instead, clearly mark it and publish a corrective release.

- **Manual Steps:**

  1. Locate the affected release in GitHub
  2. Edit the release notes and add the following warning at the top:

     ```md
     >[!WARNING]
     >PLEASE USE A NEWER VERSION (there are known issues with this release)
     ```

  3. Publish a new release that resolves the issue(s)
