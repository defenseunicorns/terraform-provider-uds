## Purpose

Ensures that `uds_package` adopts and manages only package identities that can be updated safely without silently targeting a different Zarf deployment.

## ADDED Requirements

### Requirement: Verified deployed identity
The provider MUST treat the resource ID as a lookup key and MUST verify that the deployed package returned by Zarf has the requested package name and namespace override. The provider MUST also verify that the returned package metadata name equals the returned deployed package name before persisting refreshed identity or performing a remote mutation.

#### Scenario: Returned identity matches the requested ID
- **WHEN** the provider retrieves a deployed package whose name, namespace override, and package metadata name agree with the resource ID
- **THEN** the provider continues the requested lifecycle operation using the verified returned identity

#### Scenario: Returned identity is inconsistent
- **WHEN** the returned deployed name or namespace override differs from the resource ID, or the returned package metadata name differs from the returned deployed name
- **THEN** the provider returns a blocking identity-integrity diagnostic and does not perform a package mutation

#### Scenario: Package is absent during refresh
- **WHEN** refresh confirms that the package identified by state no longer exists
- **THEN** the provider removes the resource from state according to existing missing-resource behavior

### Requirement: Canonical identity required for source-derived management
Before any update operation can remove components or deploy package content, the provider MUST verify that the existing deployed package name equals the configured source package's canonical `metadata.name`. When source package data is reloaded during an update, the provider MUST repeat this comparison against the package instance used for each removal or deployment before invoking that mutation. A mismatch MUST block component removal, deployment, state ID rewriting, and provider-managed migration or cleanup.

#### Scenario: Canonical existing package is updated
- **WHEN** the freshly verified deployed name equals the configured source package's canonical name
- **THEN** the provider proceeds with normal update behavior using that identity

#### Scenario: Imported alias differs from canonical source name
- **WHEN** the freshly verified deployed name differs from the configured source package's canonical name
- **THEN** the provider returns a blocking diagnostic before component removal or deployment and leaves both Terraform state and the deployed workload unchanged

#### Scenario: Source identity cannot be established
- **WHEN** source, architecture, transport, or package metadata needed to determine the canonical name is unavailable during a remote-mutating update
- **THEN** the provider returns a blocking diagnostic before any package mutation

#### Scenario: Source name changes between update loads
- **WHEN** a source package loaded for component removal or deployment has a canonical name different from the freshly verified deployed identity, even though an earlier source inspection matched
- **THEN** the provider blocks that mutation without invoking Remove or Deploy and does not rewrite resource identity

#### Scenario: Prior state becomes stale before update
- **WHEN** prior state identifies an existing package but the fresh pre-mutation lookup cannot find it, including when it was removed after refresh or refresh was skipped
- **THEN** the provider blocks the update instead of treating it as a create or deploying a canonical replacement

### Requirement: Plan-time canonical-name validation
When `validate_packages_on_plan` is enabled and the required configuration and refreshed prior-state identity are known, the provider MUST compare the prior deployed name with the configured source package's canonical name during planning. Planning MUST use the existing resource identity from prior state rather than unknown computed plan values or a replacement namespace, and MUST NOT require a redundant cluster lookup.

#### Scenario: Known mismatch with plan validation enabled
- **WHEN** prior state identifies an existing deployed package, source is known, package validation on plan is enabled, and the source canonical name differs from the prior deployed name
- **THEN** planning returns a blocking canonical-name diagnostic

#### Scenario: Plan validation is disabled
- **WHEN** package validation on plan is disabled
- **THEN** planning defers canonical-name comparison while apply still enforces the pre-mutation guard

#### Scenario: Required plan value is unknown
- **WHEN** source is unknown or prior-state identity is incomplete during planning
- **THEN** planning defers canonical-name comparison while apply still enforces the pre-mutation guard

#### Scenario: Namespace replacement is planned
- **WHEN** configuration plans a replacement namespace for an existing resource
- **THEN** validation locates and compares the existing package using prior-state identity rather than the planned namespace

### Requirement: Compatible import behavior
The provider MUST preserve import IDs in `name` and `namespace:name` forms and MUST keep `uds_package.name` computed. Import and refresh MUST remain read-only and MUST NOT canonicalize, rename, redeploy, take ownership of, or delete a package.

#### Scenario: Canonical package is imported
- **WHEN** an imported deployment's verified name equals the configured source package's canonical name
- **THEN** the package can proceed into normal provider management under the existing import syntax

#### Scenario: Standalone import observes an alias
- **WHEN** standalone import retrieves a package before configured source is available for comparison
- **THEN** the provider may write provisional state containing the verified deployed alias without mutating the package

#### Scenario: Provisional alias reaches configured management
- **WHEN** a later configured plan or apply compares a provisional alias with a differently named canonical source
- **THEN** the provider blocks source-derived management and directs the user to remove only the Terraform state entry and migrate externally before canonical import

### Requirement: Safe lifecycle exceptions
An explicitly allowlisted state-only update MUST preserve prior deployment-derived identity without cluster lookup, source loading, component removal, deployment, or package removal. Delete MUST freshly verify and remove the exact identity recorded in state without requiring source access or canonical-name equality.

#### Scenario: Timeout-only update
- **WHEN** an update changes only resource timeout configuration
- **THEN** the provider updates state without cluster or source access and without a Zarf mutation

#### Scenario: Delete of a canonical deployment
- **WHEN** delete freshly verifies the exact deployed identity recorded in state
- **THEN** the provider removes that verified identity without loading the configured source

#### Scenario: Delete of a provisionally imported alias
- **WHEN** a user explicitly deletes a non-canonical package whose deployed identity is freshly verified against state
- **THEN** the provider removes that exact alias identity without substituting the source package's canonical name

#### Scenario: Package is absent during delete
- **WHEN** delete confirms that the exact package identity no longer exists
- **THEN** deletion succeeds without invoking package removal

#### Scenario: Delete identity cannot be verified
- **WHEN** the returned package does not agree with the state ID or its returned package metadata name
- **THEN** the provider blocks deletion rather than risk removing a different package

### Requirement: Actionable adoption diagnostics and migration guidance
Canonical-name mismatch diagnostics MUST identify the deployed name, canonical source name, namespace override, source attribute, and required external migration action, and MUST NOT suggest configuring the computed-only `name` attribute. Diagnostics MUST NOT expose source credentials, sensitive source content, or raw package, registry, transport, or cluster error details that have not been established as safe.

Public documentation MUST distinguish non-destructive Terraform state removal from workload deletion and MUST explain why aliased deployments are unsupported. It MUST provide a guarded, operator-run migration workflow for potentially eligible ordinary-chart and raw-manifest packages. The workflow MUST require operators to inventory and back up relevant Terraform, Zarf, Helm, and Kubernetes state; reconstruct deployment inputs; classify every installed release or component; compare canonical rendering and identity with the live deployment; apply explicit stop conditions; perform an eligible canonical deployment externally; verify the handoff across state layers; remove only the exact verified stale alias Zarf Secret; and import and review the canonical provider resource.

The documentation MUST explain that takeover can claim exact Helm-rendered Kubernetes objects but cannot rename or move them. It MUST warn against using Terraform destroy, alias package removal, old-release uninstall, or direct Helm storage editing as substitutes for state removal or verified cleanup. It MUST require stale raw-manifest Helm release history to be retained after ownership transfer and MUST state that partial failures require package-specific investigation because backups do not provide a universal rollback.

#### Scenario: Canonical-name mismatch is reported
- **WHEN** the provider detects a deployed-to-canonical name mismatch
- **THEN** the diagnostic explains that management is unsupported because later Zarf operations would target a different identity and directs the user toward external migration

#### Scenario: Validation dependency returns unsafe details
- **WHEN** package, registry, transport, or cluster validation fails with an error containing credentials or sensitive source content
- **THEN** the provider returns a categorized actionable diagnostic that does not disclose the sensitive content

#### Scenario: User abandons a provisional import
- **WHEN** a user needs to preserve the deployed workload after a provisional import is rejected
- **THEN** documentation directs the user to use `tofu state rm` and warns against destroy, package removal, or Helm uninstall as substitutes

#### Scenario: Ordinary chart retains release and object identity
- **WHEN** an ordinary chart's canonical deployment retains its Helm release name and namespace and renders the same intended Kubernetes object identities
- **THEN** documentation presents it as a potential basic migration candidate after all required input, action, hook, shared-resource, and render checks pass

#### Scenario: Raw manifest changes generated release identity
- **WHEN** a raw manifest's canonical deployment uses a different package-name-derived Helm release but renders the same intended Kubernetes object identities
- **THEN** documentation explains the eligible exact-object takeover path and requires the old Helm release history to remain untouched and documented after handoff

#### Scenario: Migration hits a stop condition
- **WHEN** canonical rendering changes object identity or namespace, has incompatible selectors or immutable fields, omits resources without an explicit disposition, involves unresolved shared resources or unsafe hooks or actions, or causes multiple aliases to converge on one canonical identity
- **THEN** documentation directs the operator to stop the basic procedure and perform an application-specific migration assessment

#### Scenario: Canonical handoff is verified
- **WHEN** canonical Zarf state, Helm ownership and status, Kubernetes object identity and health, and application behavior have all been verified after external canonical deployment
- **THEN** documentation permits deletion of only the decoded, compared, and freshly backed-up stale alias Zarf Secret before canonical import

#### Scenario: Migration partially fails
- **WHEN** a migration step leaves Terraform, Zarf, Helm, or Kubernetes state inconsistent or the expected verification does not pass
- **THEN** documentation directs the operator to stop and investigate the package-specific state without claiming that restoring one backup or uninstalling one release is a safe rollback

#### Scenario: Canonical identity is imported
- **WHEN** the external handoff and exact alias-state cleanup have completed successfully
- **THEN** documentation directs the operator to import the canonical name and namespace identity, run a plan, verify canonical state, and review the first post-import deployment
