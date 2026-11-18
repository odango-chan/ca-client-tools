# Overnight Goal for DeepSeek Harness

Implement P0 in [WORK](WORK.md) on the existing draft PR #7 branch. Read [AGENTS](../AGENTS.md) first and follow its authority order and GitHub workflow. The demo target is exactly one Provider protocol and one Catalog, with no Provider compatibility requirement.

Complete P0 in the listed order. Exercise all eleven typed reads and a test-only generic SBI operation through the actual Provider process. Keep production Mutation gated. Use the shared transport and Payload Engine. Run and record the exact-head gates, including any unavailable live or GitHub checks.

Then inspect the P1 dependency in [Host integration](host-integration.md). Proceed with P1 only if the Host owner has published and pinned the accepted contract and the necessary integration is authorized. Otherwise record the exact blocker and leave P1 disabled. Keep changes within this repository and report external dependencies to their owners.

Push coherent commits to the existing head branch only. Leave the PR draft and open. Report commit SHAs, changed files, checks and evidence, live or CI limitations, and remaining work.
