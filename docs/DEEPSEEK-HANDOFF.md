# DeepSeek Harness handoff

Start at repository root. Follow the exact authority order in [AGENTS](../AGENTS.md). Use [WORK](WORK.md) for baseline, P0/P1 order and gates, and [OVERNIGHT-GOAL](OVERNIGHT-GOAL.md) as the execution prompt.

Start on draft PR #7's actual remote head. Compare it with the inspected baseline in WORK before assuming any task is complete. P0 is the single-protocol, single-Catalog read target, including end-to-end generic dispatch. P1 depends on the published Host Artifact Store contract. Record blocked dependencies and keep executing independent P0 work. Preserve operation semantics, typed validation and Host/Runtime authority. Use dated evidence for earlier CLI, transport and Provider results.

Deliver exact head SHA, changed files, verified gates and unresolved blockers. Follow the GitHub workflow in AGENTS.
