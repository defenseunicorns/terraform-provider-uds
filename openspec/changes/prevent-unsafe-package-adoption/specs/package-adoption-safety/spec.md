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
Before any update operation can remove components or deploy package content, the provider MUST verify that the existing deployed package name equals the configured source package's canonical `metadata.name`. A mismatch MUST block component removal, deployment, state ID rewriting, and provider-managed migration or cleanup.

#### Scenario: Canonical existing package is updated
- **WHEN** the freshly verified deployed name equals the configured source package's canonical name
- **THEN** the provider proceeds with normal update behavior using that identity

#### Scenario: Imported alias differs from canonical source name
- **WHEN** the freshly verified deployed name differs from the configured source package's canonical name
- **THEN** the provider returns a blocking diagnostic before component removal or deployment and leaves both Terraform state and the deployed workload unchanged

#### Scenario: Source identity cannot be established
- **WHEN** source, architecture, transport, or package metadata needed to determine the canonical name is unavailable during a remote-mutating update
- **THEN** the provider returns a blocking diagnostic before any package mutation

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
Canonical-name mismatch diagnostics MUST identify the deployed name, canonical source name, namespace override, source attribute, and required external migration action, and MUST NOT suggest configuring the computed-only `name` attribute. Diagnostics MUST NOT expose source credentials, sensitive source content, or raw package, registry, transport, or cluster error details that have not been established as safe. Public documentation MUST distinguish non-destructive state removal from workload deletion, explain why aliased deployments are unsupported, and provide high-level migration safety considerations without presenting an unvalidated prescriptive procedure or guaranteed provider operation.

#### Scenario: Canonical-name mismatch is reported
- **WHEN** the provider detects a deployed-to-canonical name mismatch
- **THEN** the diagnostic explains that management is unsupported because later Zarf operations would target a different identity and directs the user toward external migration

#### Scenario: Validation dependency returns unsafe details
- **WHEN** package, registry, transport, or cluster validation fails with an error containing credentials or sensitive source content
- **THEN** the provider returns a categorized actionable diagnostic that does not disclose the sensitive content

#### Scenario: User abandons a provisional import
- **WHEN** a user needs to preserve the deployed workload after a provisional import is rejected
- **THEN** documentation directs the user to remove the Terraform state entry without deleting the workload

#### Scenario: Migration eligibility is uncertain
- **WHEN** package names affect rendered resources, release identity, namespace, shared resources, hooks, or package actions
- **THEN** documentation warns that external migration requires package-specific investigation, backup, and verification before canonical import and identifies detailed procedures as follow-on guidance
