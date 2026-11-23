# Scheme C typed MAS callables (UE Modify / Release)

Status: **Development Provider binding, exercise-tested against the merged
OpenAMSM Owner (`93fcace…`)**. No Owner ToolCatalog, commit receipt or
exact-original Reconcile publication is claimed.

## Why

The merged Scheme C Owner performs UE-requested PDU Session Modification and
Release as native SMF transactions:

- `SSM.SIGNAL_UE_AN` decodes the original 5GSM N1 (`parse_n1=1`), builds the
  Release Command (`mode=build`) and confirms the N1/N2 completion
  (`release_confirm=n1|n2`);
- `UPN.MODIFY_UP_SESSION` applies the typed QoS delta (`qos_flows_delta=1`);
- `UPN.DELETE_UP_SESSION`, `PA.DELETE_SM_POLICY`, `SE.EMIT_EVENTS` and
  `SC.RELEASE_SM_CONTEXT` perform the release cleanup.

The older OES-shaped `m0/m3/m4/m5` and `r2–r6` callables remain addressable but
are **not** the Scheme C execution order; a Procedure that uses them is not
executing this Owner transaction.

## Added callables

| Callable | Occurrence | Owner MAS/op | Inputs (wire) | Outputs (wire) |
|---|---|---|---|---|
| `SSM.PARSE_N1_MODIFY` | `c-parse` | `SSM/SIGNAL_UE_AN` | `parse_n1=1`, `n1_sm` | `qfi`, optional `five_qi/gfbr_*/mfbr_*`, `requested_kind` |
| `UPN.MODIFY_QOS_DELTA` | `c-apply` | `UPN/MODIFY_UP_SESSION` | `qos_flows_delta=1`, `qfi`, `five_qi`, `gfbr_ul/dl`, `mfbr_ul/dl` | `qos_delta_state`, optional `n1n2_dispatched`, target fields |
| `SSM.PARSE_N1_RELEASE` | `r-parse` | `SSM/SIGNAL_UE_AN` | `parse_n1=1`, `n1_sm` | `psi`, `pti`, optional `cause`, `release_intent` |
| `SSM.BUILD_RELEASE_COMMAND` | `r-build` | `SSM/SIGNAL_UE_AN` | `release=1`, `mode=build`, `trigger`, `release_txn` | `n1_sm_hex`, `n2_sm_hex`, `n2_sm_info_type`, `release_generation`, `release_txn`, `release_phase` |
| `SSM.RELEASE_CONFIRM_N1` | `r-confirm-n1` | `SSM/SIGNAL_UE_AN` | `release_confirm=n1`, `release_txn`, `release_generation` | `release_ready`, `n1_released`, `n2_released` |
| `SSM.RELEASE_CONFIRM_N2` | `r-confirm-n2` | `SSM/SIGNAL_UE_AN` | `release_confirm=n2`, `release_txn`, `release_generation` | `release_ready`, `n1_released`, `n2_released` |
| `PA.DELETE_SM_POLICY_C` | `r-pa` | `PA/DELETE_SM_POLICY` | `release=1`, `trigger` | `terminated` |
| `SC.RELEASE_SM_CONTEXT_C` | `r-sc` | `SC/RELEASE_SM_CONTEXT` | `release=1`, `finalize=1`, `trigger` | `lifecycle` |

`UPN.DELETE_UP_SESSION` (`r2`) and `SE.EMIT_RELEASE_EVENT` (`r6`) already
existed and are reused with `trigger=2` / `ev=RELEASED`.

## Safety properties enforced here

- The exact `mas`, `op` and occurrence `stepId` are fixed in source; a caller
  can only select the callable.
- `ue_identity`/`psi` are Host scope, checked against the invocation and never
  forwarded as Owner inputs.
- The N1 payload is opaque hex (even length, bounded); no NAS is parsed by the
  Provider.
- QoS delta values must be canonical decimal, non-zero, `gbr <= mbr`.
- Outputs are closed: every declared field must be present (or explicitly
  optional) and undeclared fields are rejected.
- The two parse occurrences are `READ`; all other additions are `MUTATION`.

## Not claimed

- Owner ToolCatalog / Evidence Guarantee publication for these callables.
- Owner commit receipt / exact-original Reconcile.
- Production admission of a compiled Skill that uses them.
