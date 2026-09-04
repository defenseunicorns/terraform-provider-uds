## 1. Identity Validation Foundation

- [x] 1.1 Add a deployed-package identity representation and typed errors that distinguish absence, remote identity corruption, prior-state inconsistency, and canonical-name mismatch; verify focused unit tests assert each error classification and diagnostic context.
- [x] 1.2 Implement remote identity lookup that parses `name` and `namespace:name`, retrieves the exact Zarf package, and verifies returned name, namespace override, and `Data.Metadata.Name`; verify unit tests cover canonical identity, each mismatch dimension, malformed IDs, not-found responses, and cluster errors.
- [x] 1.3 Implement prior-state identity validation for known ID, computed name, and namespace values without cluster access; verify unit tests cover consistent state, stale name, stale namespace, null or incomplete imported state, and namespace-qualified IDs without constructing impossible unknown persisted state.
- [x] 1.4 Implement canonical source-name comparison using metadata-only package loading and allowlisted diagnostic context; verify unit tests cover matching names, aliases, source/load failures, architecture and transport failures, package-layout cleanup, and sentinel secrets absent from surfaced diagnostics.

## 2. Read And Import Safety

- [x] 2.1 Route `Read` through remote identity verification before writing computed state while preserving confirmed-absence state removal and prior state on other errors; verify resource tests cover canonical refresh, provisional alias refresh, inconsistent returned identity, missing packages, and error responses that do not write partial state.
- [x] 2.2 Keep `ImportState` as ID passthrough and add framework coverage for both import ID forms; verify `Read` hydrates remotely recoverable state, a canonical first configured plan/apply may redeploy but targets the same verified identity, and alias import remains read-only and performs no Deploy, Remove, alias mutation, or canonicalization before configured rejection.
- [x] 2.3 Route Create post-deployment refresh and failed-deployment recovery through expected canonical identity verification; verify an inconsistent returned package name, namespace override, or metadata name produces an identity-integrity diagnostic and does not persist refreshed or recovered state, while verified failed-deployment recovery continues to preserve state alongside the original Create error.

## 3. Plan-Time Validation

- [x] 3.1 Extend package-dependent plan checks to compare source canonical name with verified prior-state identity in the same metadata load used by existing checks; verify tests show one package load and no cluster lookup when name, signature, optional-component, or value checks run together.
- [x] 3.2 Wire existing-resource `ModifyPlan` to use prior ID/name/namespace and emit identity or canonical mismatch diagnostics when package validation on plan is enabled; verify framework tests cover a canonical update, alias mismatch, stale state, and planned namespace replacement.
- [x] 3.3 Preserve validation deferral when package validation is disabled, source is unknown, or prior identity is null or incomplete, and preserve the early state-only plan path; verify tests assert no package load or cluster call in each deferred or state-only case without placing unknown values in persisted state.

## 4. Authoritative Update Guard

- [x] 4.1 Freshly verify remote identity and canonical source name under the update timeout after state-only detection and before `deployAsNewOrUpdate`; verify tests assert aliases, missing packages, inconsistent metadata, and source failures cannot call component filtering, Remove, or Deploy and leave response state untouched so prior state is retained.
- [x] 4.2 Pass the verified existing identity into the update mutation boundary and reject missing or mismatched identity before component-removal calculation or upsert; verify focused tests prove both legacy and optional-component removal paths remain unreachable on validation failure.
- [x] 4.3 Preserve canonical Create and Update behavior, computed state identity, shared timeout budgets, and state-only timeout updates; verify existing lifecycle tests plus new canonical and timeout-only regression tests pass without extra cluster or source calls.
- [x] 4.4 Revalidate the canonical name of every source package instance used by component removal or deployment against the verified existing identity; verify a name change between loads cannot call Remove or Deploy or rewrite state identity.

## 5. Exact Delete

- [x] 5.1 Replace Delete's source-or-cluster reconstruction with fresh exact-identity retrieval and removal using verified returned package data and namespace; verify tests cover canonical deletion, alias deletion without source access, namespace-qualified identity, and remaining Zarf timeout budget.
- [x] 5.2 Treat confirmed absence as successful deletion while blocking lookup and identity-integrity failures; verify tests assert Remove is skipped for absence or failure, failed Delete leaves state for retry, and successful or already-absent Delete returns without writing state or calling `RemoveResource` so the Plugin Framework removes state automatically; verify Remove is called only once with the exact package when present.

## 6. User Guidance

- [x] 6.1 Add canonical-name and identity-integrity diagnostics that include deployed name, canonical name, namespace, the `source` attribute path, and external migration action without echoing potentially sensitive source or dependency error content or suggesting a configurable `name`; verify diagnostic tests assert required safe details, sentinel-secret non-disclosure, and no mutation after an error.
- [x] 6.2 Update `uds_package` import documentation for canonical eligibility, provisional standalone state, `tofu state rm`, exact Delete semantics, and both import ID forms; run `uds run generate` and verify generated documentation contains the updated guidance.
- [x] 6.3 Add concise guidance explaining why aliased deployments are unsupported, the risks of destructive cleanup or unvalidated takeover, and the need for package-specific investigation, backup, and verification before canonical import; verify it avoids prescriptive migration commands and identifies detailed tested procedures as follow-on work.

## 7. End-To-End Verification

- [x] 7.1 Add protocol-v6 acceptance coverage that builds an isolated package fixture, deploys it through a UDS bundle using UDS CLI whose package entry name differs from source `metadata.name`, and then exercises configuration-driven import blocks and standalone import, provisional state removal, pre-mutation rejection, canonical import hydration, and canonical post-import redeployment of only the same identity; verify the focused OpenTofu disposable-cluster suite passes, no second package identity is created, and alias state and cluster artifacts not owned by the final Terraform state are explicitly cleaned up.
- [x] 7.2 Run `uds run lint:check` and `uds run test-unit`, fix any regressions, and verify both commands complete successfully.
- [x] 7.3 Run the applicable `uds run test-acc` suite for package lifecycle behavior and verify canonical create/import/update/delete remains successful while non-canonical management is blocked before mutation.
