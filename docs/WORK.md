# Work

## Verified starting point

On 2026-11-18, [draft PR #7](https://github.com/odango-chan/ca-client-tools/pull/7) targets `main` from `feat/unified-provider-read-tools-20261117`. The inspected code head is `6f547b4ba0b1f6deb847e1a041416a2749540a06`.

- The resident Provider and embedded Catalog still have two code paths/artifacts. The eleven typed OpenAMSM read bindings are implemented. MAS uses shared SBI transport. The metrics handlers retain their source-specific HTTP behavior.
- The generic Catalog loader/bridge, executor, schema checker, trusted configuration loader and Payload Engine exist. The Provider Runtime switches among typed handlers only. Generic SBI through the Provider, consolidated wire validation and Host artifact integration are implementation work.
- The [Provider acceptance record](evidence/provider-acceptance-20261117.md) records historical live results for the typed paths at its pinned revision. Earlier CLI/transport tests are in [Windows/WSL evidence](evidence/windows-wsl-acceptance-20261117.md). Neither establishes acceptance for the consolidated target.
- The historical PR #7 [Actions run](https://github.com/odango-chan/ca-client-tools/actions/runs/44101812285) did not start due to billing/spending limits. Record GitHub CI status separately for the actual implementation head. No runtime tests were run for this documentation pass.

## P0 — consolidate and connect read execution

1. Update protocol, Catalog embedding, tests and acceptance runner together. Accept exactly one protocol ID and load exactly one embedded Catalog. Remove legacy branching, loaders, artifacts and compatibility checks. Preserve the eleven read bindings and typed behavior.
2. Implement recursive validation of nested invocation arguments against trusted schemas. Use one Catalog-driven dispatch path. Wire `genericSBI` through the existing Catalog bridge, trusted service configuration, executor, shared transport and Payload Engine. Retain typed MAS identity/ProblemDetails behavior and typed metrics paging and error behavior. Consolidate transport and payload plumbing while keeping h2c prior knowledge for SBI and explicit HTTP/1.1 for source-verified metrics.
3. Keep deployed bindings read-only. Prove Generic SBI through `provider serve` with a test-only trusted binding and deterministic loopback upstream. Test nested objects and arrays, precision and defaults, wrong shape and duplicate keys, bounds, trusted routing, status/ProblemDetails, timeout/cancellation, and fail-closed production Mutation. No Host artifact dependency for P0.

P0 closes when one protocol and one Catalog are observable in the binary and acceptance runner, the legacy ID is rejected, typed MAS and metrics semantics pass regression tests, and a process-level generic invoke demonstrably reaches the executor through trusted configuration. Record format, test, race, vet, native build and process acceptance at the exact head. Re-run all eleven typed reads against the pinned lab on Windows-native and Linux-native when available. Report blocked or unavailable gates separately, including CI and live lab access.

## P1 — Host artifact streaming

After P0 and publication of the [Host contract](host-integration.md), connect invocation-scoped readers and sinks, add reference handling at declared slots, and verify the complete [payload acceptance](provider-payload-extension-v1.md). Keep P1 marked blocked until the Host contract has an accepted URL and commit. Mock executor fixtures alone do not complete P1.

## Production Mutation gate

Keep the deployed Catalog read-only and reject Mutation before remote I/O. Mutation fixtures may exercise metadata and rejection paths in isolation. Enabling a concrete operation requires pinned authoritative source/deployment evidence, explicit Host and Runtime authorization, operation-specific idempotency/reconciliation semantics and reviewed acceptance. An idempotency key alone authorizes neither dispatch nor retries. No automatic retry or implicit polling.
