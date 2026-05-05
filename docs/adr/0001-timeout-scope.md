# ADR 0001: Scope of the `timeout` attribute on `uds_package`

## Status

Accepted, 2026-05-05.

## Context

Before this change (CLI-173), the `timeout` attribute on `uds_package` was only forwarded to Zarf's helm timeout. As a result, `terraform apply` could exceed the configured value, because cluster connection wait and OCI package pull/load were not bounded by it.

User feedback (Slack, [#cli](https://defense-unicorns.slack.com/archives/C08KECJFRDF/p1777359840015259)):

* Jase Koonce (customer): "I personally expected it to cover the whole process (pull -> deploy)."
* Joel McCoy: "I should never see a terraform resource log 5m10s when the deploy timeout is 5m."

The fix is small in code but a UX/architecture call. This ADR records the chosen scope so future readers do not have to re-litigate it.

## Decision

The `timeout` attribute bounds the **entire** Create, Update, or Delete operation as a single wall-clock budget, including:

1. Cluster connection wait (`withClusterTimeout` is now a child of the operation timeout).
2. OCI/disk package load and pull (`GetPackageFromSourceOrCluster`, `getPackageLayoutFromSource`).
3. Zarf deploy or remove (`packager.Deploy`, `packager.Remove`).

Implemented via a `context.WithTimeout` derived from `plan.Timeout` (or `data.Timeout` for Delete) and threaded through the framework callbacks. The same duration is also forwarded into Zarf's `DeployOptions`/`RemoveOptions` as a second line of defense.

## Rationale

* Matches user expectation (see Context).
* A single `timeout` attribute can only sanely express a wall-clock budget. Partial scoping leaves cluster wait and OCI pull unbounded, which is exactly the failure mode CLI-173 reports.
* Smallest change that fixes the issue without breaking the existing `timeout` field.

## Alternatives considered

1. **Narrow wrap, only `packager.Deploy`/`packager.Remove`.** Rejected. A slow registry or flaky API server can still blow past the configured `timeout`, defeating the fix; conflicts with Joel's wall-clock principle above.
2. **Custom nested attribute, e.g. `package_operation_timeouts = { pull, deploy, total }`.** Rejected. No explicit user demand for per-phase tuning, increases schema complexity, and Terraform Plugin Framework's nested `timeouts` block does not support custom keys (only `create`/`read`/`update`/`delete`).
3. **Standard Plugin Framework `timeouts { create, update, delete }` block.** Rejected for now. Also lacks explicit user demand and would require deprecating or removing the existing `timeout` attribute. Recorded as the most idiomatic future evolution if per-operation tuning is later requested.

## Consequences

* On timeout, the operation is canceled and Terraform returns an error. The package may be partially installed/removed in the cluster; reconciling that divergence is out of scope here and tracked by [CLI-101](https://linear.app/defense-unicorns/issue/CLI-101) (drift detection) and [CLI-130](https://linear.app/defense-unicorns/issue/CLI-130) (non-cluster Zarf packages).
* The internal `withClusterTimeout` 5 min cap remains, but it is now a child of the operation timeout, so the user-configured timeout always wins if it is shorter.
* If users later ask for per-operation control, alternative (3) is the path forward.
