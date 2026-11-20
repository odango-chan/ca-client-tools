# MAS v0.3.0 publication naming

The stable package identity remains `wtlab:packages:openamsm:mas-compiler-contract`. The declared Owner / Publisher label remains `wtlab`; it is not proof of publisher authorization.

## Version policy

- v0.2.0 remains immutable and contains the 13 Establishment callables and 25 Schema projections.
- v0.3.0 adds 8 lifecycle callables and 15 new Schema projections.
- The 25 unchanged v0.2.0 Schema definitions are inherited byte-identically. Their existing `wtlab:openamsm:mas:v0.2.0:` IDs are retained.
- New lifecycle projections use `wtlab:openamsm:mas:v0.3.0:` IDs.
- The v0.3.0 package contains one combined 21-callable candidate closure and 40 distinct Schema IDs.

No existing Schema ID is reassigned or widened. In particular, the v0.2.0 `UPN.MODIFY_UP_SESSION` and `SSM.SIGNAL_UE_AN` result Schemas are reused only where the exact output shape matches. New inputs and different output shapes receive new IDs.

## New Schema IDs

| Profile | Input ID suffix | Output ID suffix |
| --- | --- | --- |
| SC Update SM Context | `sc-update-sm-context-input` | `sc-update-sm-context-result` |
| UPN Modify Intent | `upn-modify-intent-input` | `upn-modify-intent-result` |
| SSM Modify AN | `ssm-signal-ue-an-modify-input` | Reuses v0.2.0 `ssm-signal-ue-an-result` |
| UPN Delete UP Session | `upn-delete-up-session-input` | `upn-delete-up-session-result` |
| SSM Release AN | `ssm-signal-ue-an-release-input` | `ssm-signal-ue-an-release-result` |
| PA Delete SM Policy | `pa-delete-sm-policy-input` | `pa-delete-sm-policy-result` |
| SC Release SM Context | `sc-release-sm-context-input` | `sc-release-sm-context-result` |
| SE Release Event | `se-emit-release-event-input` | `se-emit-release-event-result` |

The empty output Schemas for SSM Release, PA Delete and SE Release Event are closed empty objects. They intentionally contain no synthetic business fields.

## Download and trust

The archive filename is `openamsm-mas-compiler-contract-v0.3.0.zip`. It is a digest-pinned, non-linkable candidate archive hosted as a ca-client-tools release asset. A GitHub release asset does not establish WTLab publication authority. Deployment must independently authorize the real publisher principal and bind its verified credentials and scope to the exact package digest.
