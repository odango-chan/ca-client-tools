# OpenAMSM SMF Query Backend Contract v1

**Status:** BACKEND CONTRACT — FROZEN FOR IMPLEMENTATION  
**Public CA-SL Tool Owner:** ca-agent-host  
**Backend implementation Owner:** ca-client-tools  
**Fact authority:** SMF / OpenAMSM  
**Implementation status:** Typed Go client exists; resident Provider backend NOT IMPLEMENTED.

This document defines only the private Host-to-Provider query backend. It does not publish Skill-facing ToolCatalog callables, Capability IDs, Evidence Descriptors or Agent-visible Output Schemas. Those are Host-owned contracts in [ca-agent-demo PR #208](https://github.com/odango-chan/ca-agent-demo/pull/208) and the Host Runtime integration contract in [PR #214](https://github.com/odango-chan/ca-agent-demo/pull/214).

## 1. Ownership boundary

| Concern | Owner |
|---|---|
| Skill-visible generic session-context Observation context.smf.session.read | ca-agent-host |
| Eight optional projection-specific Observation callables | ca-agent-host |
| ToolCatalog, normalized Schemas, Owner Outcomes and Evidence Producer | ca-agent-host |
| CapabilityContracts and Scope policies | Deployment |
| Provider process, backend operation IDs and typed Go client | ca-client-tools |
| Network fact and raw QueryContext response | SMF / OpenAMSM |

The Host adapter maps public callable identities to private Provider backend operation IDs. The Provider backend catalog is not a second ToolCatalog and is not an authorization source.

## 2. Private backend operation IDs

The Provider supports these exact backend operation IDs:

| Provider backend operation ID | Fixed MAS | OpenAMSM route |
|---|---|---|
| openamsm.smf.mas.sc.query | SC | POST /nsmf-sc-context/v1/query |
| openamsm.smf.mas.pa.query | PA | POST /nsmf-pa-context/v1/query |
| openamsm.smf.mas.upn.query | UPN | POST /nsmf-upn-context/v1/query |
| openamsm.smf.mas.ssm.query | SSM | POST /nsmf-ssm-context/v1/query |
| openamsm.smf.mas.iam.query | IAM | POST /nsmf-iam-context/v1/query |
| openamsm.smf.mas.ups.query | UPS | POST /nsmf-ups-context/v1/query |
| openamsm.smf.mas.ca.query | CA | POST /nsmf-ca-context/v1/query |
| openamsm.smf.mas.saa.query | SAA | POST /nsmf-saa-context/v1/query |

The backend operation ID selects one fixed MAS and route. The caller cannot supply a MAS selector, route, URL, method, service key or header. The Provider maps the ID to internal/openamsm/smf/mas.Client.Query with the corresponding constant Query.MAS.

## 3. Backend Query arguments

The Provider process accepts exactly:

- supi: formatted SUPI, imsi- followed by 1–15 decimal digits;
- pduSessionId: integer 0–255.

The Skill-facing Host Observation QueryContext Schema narrows PSI to the 3GPP PDU Session ID range 1–15 and the admitted SUPI profile. The Provider/CLI typed client retains its broader OpenAMSM API validation. IMSI alias and UE IPv4/IPv6 lookup forms remain CLI/operator capabilities and are not exposed to Skill Procedures through this Provider profile.

The Host must bind the query SUPI to the admitted Skill Subject Context and bind the PSI to the HostBusinessInstance for the current UE/PSI lifecycle. The private HostBusinessCorrelation (ingressRef, lifecycleRef) remains outside Skill arguments and outside the Provider QueryContext. Provider transport success does not prove Host Scope or lifecycle authorization.

## 4. Raw response semantics

The Provider returns the typed remote response to the Host adapter; it does not map CA-SL Owner Outcomes.

### HTTP 200

The OpenAMSM response contains exactly:

- masId: the fixed MAS short name selected by the backend operation;
- status: AVAILABLE or UNAVAILABLE;
- key: exact echoed SUPI and PDU Session ID for the identity lookup;
- context: the MAS projection.

The existing typed Go client validates the returned MAS ID, status, exact echoed SUPI/PSI key and UNAVAILABLE empty-context rule. The Host adapter additionally validates the exact normalized projection Schema and status-dependent required fields, then removes the private masId and raw key from the Agent-visible output.

- AVAILABLE: the selected MAS has authoritatively written its own State and returns its projection. False business values remain valid values.
- UNAVAILABLE: the session exists, but the selected MAS has not authoritatively written its State, or the bundle is absent. The context MUST be exactly {}. It is not absence and not a negative business fact.

### HTTP 404

SESSION_NOT_FOUND means the current SMF UE/PSI lookup did not find a matching session. It is distinct from HTTP 200 UNAVAILABLE.

The generic Host callable context.smf.session.read normalizes only exact HTTP 404 + SESSION_NOT_FOUND into sessionPresence: ABSENT, projectionStatus: NOT_APPLICABLE, sourceOutcome: SESSION_NOT_FOUND, and context: {}. The output carries the admitted SUPI and PSI from the validated QueryContext; it does not claim that the error response echoed them. This is a business result backed by the exact query invocation and Host Scope binding.

The eight projection-specific Host callables do not map 404 to UNAVAILABLE; their Owner adapter returns the declared Owner failure for SESSION_NOT_FOUND. A Skill needing an explicit presence/absence decision uses the generic callable.

### Other ProblemDetails

The OpenAMSM handler declares these safe machine causes: INVALID_QUERY_KEY, INVALID_PARAM, SESSION_NOT_FOUND, KEY_CONFLICT, INTERNAL, FORBIDDEN, UNKNOWN_QUERY_SERVICE, METHOD_NOT_ALLOWED, and SERVICE_UNAVAILABLE.

A valid remote ProblemDetails response is returned as a typed remote HTTP result by the Provider. Host maps it according to the public Host Owner contract; it is never Host Runtime DENIED merely because the remote SMF returned HTTP 403. Local Capability/Scope rejection occurs before Provider dispatch and maps to Runtime DENIED.

Transport failure, malformed ProblemDetails, wrong MAS type, invalid 200 body or mismatched echoed key is not a valid remote business result. The Provider returns a protocol/backend error; Host/Runtime maps it to UNKNOWN and MUST NOT fabricate Owner failure or Evidence.

## 5. Typed client reuse and constraints

The Provider MUST reuse internal/openamsm/smf/mas.Client.Query. It MUST NOT invoke ca-client-tools smf context query as a subprocess and MUST NOT copy/reimplement the h2c client, SUPI/PSI validation, echoed-key binding or ProblemDetails handling.

The current client uses h2c HTTP/2 prior knowledge, no redirect following, a 5-second default timeout (30-second maximum), a 1 MiB default response limit (4 MiB maximum), and X-OpenAMSM-Query-Token. The token and service origin are trusted deployment configuration only.

The current Go client returns typed RemoteError for non-200 responses. The Provider implementation MUST ensure that only a valid ProblemDetails response (correct media type, JSON object, status matching HTTP status, safe cause) is returned as a remote result; malformed error bodies must be surfaced as REMOTE_RESPONSE_INVALID. This is an explicit Provider implementation acceptance requirement.

## 6. Freshness and qualification boundary

The pinned SMF v0.10 response contains no observedAt, eventTime or validUntil. Host queryStartedAt and receivedAt are audit timestamps, not SMF source timestamps. Neither the Provider nor Host may relabel them as observedAt, manufacture validUntil or invent maxAgeMs.

The current response alone cannot satisfy the CA-SL v1alpha4 Evidence freshness requirement. The Host Owner's generic session-context Evidence Descriptor and eight projection-specific Evidence Descriptors remain NOT_PUBLISHED / NOT_LINKABLE / NOT_QUALIFIED until the SMF Owner publishes either a source observation timestamp or an explicit testable live-authoritative-read guarantee that supports a conservative freshness calculation.

This freshness gate does not prevent Provider implementation, raw query acceptance or Host transport integration. It prevents Evidence-dependent Predicate PASS from being qualified on these results.

## 7. Source pin

- Repository: odango-chan/env-openamsm
- Commit: 594dec78b95bff4fdc34deac8a3a9681fb41228f
- OpenAPI: upstream/openamsm/tools/nw-agent/openapi/Nsmf_MasContextQuery.yaml
- OpenAPI blob: 46045c2f8f1cae31c6e6c98d5d20addbebe41969

The eight projection implementations are under upstream/openamsm/src/smf/mas/{sc,pa,upn,ssm,iam,ups,ca,saa}/; shared resolution and echo behavior are under upstream/openamsm/src/smf/mas/common/.
