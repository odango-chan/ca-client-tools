# CA Client Tools architecture

## 1. Purpose

CA Client Tools provides typed local client operations for the interfaces
already exposed by the selected OpenAMSM environment.

It is a client integration layer, not a second 5GC implementation, not an
Agent, not a Runtime, and not an OpenAMSM environment manager.

## 2. Component boundaries

The stable NF/capability vocabulary, 3GPP → Tool → Skill derivation rule and
source-backed AMF/SMF/UPF availability matrix are maintained in
[docs/nf-capability-map.md](nf-capability-map.md).

```text
Skill Procedure / Agent request
          |
          v
ca-runtime
  callable admission + Capability authorization
          |
          v
ca-agent Host trusted adapter
  fixed executable + typed arguments + trusted configuration
          |
          v
ca-client-tools CLI
  OpenAMSM client implementation
          |
          +---- HTTP GET ----> OpenAMSM AMF metrics endpoints
          |
          +---- HTTP GET ----> OpenAMSM SMF metrics endpoint
          |
          +---- HTTP GET ----> OpenAMSM UPF /session-info (CA patch overlay)
          |
          +---- HTTP/SBI POST -> OpenAMSM SMF context-query adapter
```

The exact Runtime-to-Host dispatch seam remains governed by the paired
ca-agent / ca-runtime contracts. This repository does not invent a second
execution ingress.

The CLI is only the local process boundary. It does not replace the protocol
provided by OpenAMSM. In particular, SMF context queries remain remote
HTTP/SBI POST with the OpenAMSM query-token header.

## 3. Responsibilities

### This repository

- OpenAMSM endpoint clients.
- Request validation and response-envelope validation.
- Bounded HTTP transport.
- A stable, non-interactive CLI process interface defined by `docs/cli-contract.md`.
- Focused client and CLI contract tests using HTTP fixtures.

### Not this repository

- OpenAMSM source, build, configuration or process lifecycle.
- Agent planning or Skill selection.
- Agent-facing MCP.
- Runtime callable admission or Capability authorization.
- Runtime requestId/runId/invocationId or Action identity.
- Host ProductTask, AgentInstance or HostBusinessInstance.
- Predicate evaluation, Evidence authority, Outcome or Reconcile.
- Provider mutation workflows, Fabric journals or callback ownership.

## 4. Agent-callable versus Host-internal

The existence of a CLI subcommand does not make it Agent-callable.

The Host owns the integration and exposure policy. Runtime owns callable
admission and Capability authorization.

Initial exposure intent:

| Client operation | Intended consumer | Agent-callable by default |
|---|---|---|
| SMF context query (session/policy/up-node/signaling-session/ip-address/up-selection/charging/session-auth) | Agent through declared Tool/Capability | Yes, subject to Runtime admission |
| AMF `/ue-info` | Host context/default-tool integration | No |
| AMF `/gnb-info` | Host context/default-tool integration | No |
| SMF `/pdu-info` | Host context/default-tool integration | No |
| UPF `/session-info` (CA-owned source patch overlay) | Host/Tool internal integration | No |

The Host may use internal default tools without exposing them as Agent-callable
Tools. The CLI itself does not enforce this visibility policy.

The OpenAMSM SMF context-query wire response echoes the resolved `supi` (when available),
`pduSessionId` and supplied UE IP keys. `imsi` is a request-only alias and is
not a response-key property. For an Agent-callable Observation, the Host
resolves the Skill's opaque `ueRef` to a trusted SUPI/IMSI before invoking the
CLI, normalizes an IMSI binding to SUPI for response comparison, verifies the
echoed key, and redacts/replaces the raw subscriber identity before returning
the normalized result to the Agent. Raw CLI stdout is private Host input, not
the Agent-visible Observation payload.

## 5. OpenAMSM-only source policy

OpenAMSM is the sole target. The repository must not maintain a parallel
Open5GS backend or compatibility package. The UPF session projection is a
CA-owned patch overlay maintained in `env-openamsm`; this client repository
consumes its contract and never edits the immutable imported source.

The current source inspection identifies AMF `GET /ue-info` and `GET /gnb-info`, SMF `GET /pdu-info` and eight context-query SBI endpoints. The immutable UPF source has no `GET /session-info`; the separate CA-owned patch overlay adds a read-only projection from UPF PFCP session state.

OpenAMSM SMF `/pdu-info` does not provide the SMF/UPF N4 SEID pair supplied
by the old Open5GS-specific patch. UPN context `n4Seid` is a single UPF SEID and
must not be reinterpreted as that pair.

The client does not port the old Open5GS-specific patch. Instead, the
`env-openamsm` overlay reads the exact UPF-side N4 SEID and SMF F-SEID already
stored in the UPF PFCP session. It also projects UE IP, DNN, PDR/FAR/QER/URR
state. This is a CA-owned internal diagnostic extension, not a standardized
3GPP SBI service. It must be applied to a disposable source copy, built and
accepted against a live N4 session before deployment use.

## 6. Initial implementation slice

The implementation retains the read-only SMF context-query baseline, AMF
`/ue-info` and `/gnb-info` page clients, and SMF `/pdu-info` page client.
It adds a UPF `/session-info` client and `upf session list` CLI command that
depend on the separate `env-openamsm` patch overlay. All metrics clients
validate the source pagination envelope. AMF and UPF results remain
Host/Tool-internal by default; the existing SMF context-query exposure rule is
unchanged.

The CLI contract is defined in `docs/cli-contract.md` (CLI v1). It selects
Cobra for command registration/help/flag parsing, keeps Cobra out of the
OpenAMSM client packages, uses JSON stdin for sensitive SMF context lookup values,
wraps successful results in `data`, emits errors on stderr, and uses stable
exit-code classes. The public response uses the CA-facing context `type`; the
OpenAMSM wire `masId` is validated and kept private to the adapter. The implemented command tree is `amf ue list`, `amf gnb list`,
`smf pdu list`, `smf context query` and `upf session list`. UPF defaults to
page size 20; AMF/SMF metrics default to 100. Go 1.26.2 CI, UPF patch build and
runtime acceptance remain pending.

The client must preserve:

- the eight exact OpenAMSM internal route identifiers and paths (private adapter detail);
- identity-plus-PSI or UE-IP query-key rules;
- `X-OpenAMSM-Query-Token` authentication;
- `AVAILABLE` versus `UNAVAILABLE` response semantics;
- `404 SESSION_NOT_FOUND`, `403 FORBIDDEN`, `503 SERVICE_UNAVAILABLE`,
  and ProblemDetails errors.

A successful `UNAVAILABLE` response is not a transport error and is not a
business negative.

The AMF and SMF metrics response envelopes and paging rules are source-verified.
The UPF patch uses the same `items` plus `pager` envelope, with default page
size 20 and maximum 100. Typed clients and CLI commands for AMF UE, AMF gNB,
SMF PDU and UPF PFCP session listings are implemented on the review branch.
Fixture coverage is authored; execution, Go 1.26.2 validation, patched
OpenAMSM build and live-N4 acceptance remain pending.

## 7. Source and acceptance status

The authoritative source snapshot is imported under
`env-openamsm/upstream/openamsm` on `env-openamsm/main` (archive SHA-256
`6b61b330c15e01464039fed3b916507f94f760303fb8aeedf9b9a8bfa380547f`, source
commit comment `c27e1761`). The immutable source inspection is static only.
The UPF extension is a separate patch overlay; patch application, native
build, source tests and live N4 queries remain NOT_RUN. Metrics endpoints are plain
HTTP GET handlers on the NF metrics listener; this handler does not apply
application authentication, so endpoint origins must remain trusted and
network-restricted by deployment.


## 8. Adjacent NW Agent APIs are not part of this client scope

The pinned OpenAMSM source snapshot also contains separate NW Agent-facing
interfaces. Their presence does not expand this repository's read-only client
scope:

| Interface | Source contract | Decision in `ca-client-tools` |
|---|---|---|
| Workflow Planning / Replan | `Nnwagent_WorkflowPlanning.yaml` | Excluded; workflow-plan service contract, not a context query |
| Nsmf PDUSession facade | `Nnwagent_NsmfFacade.yaml` | Excluded; NF facade/relay behavior |
| SM Context orchestration/update synchronization (private source contract: `Nsmf_MasOrchestration.yaml`) | `Nsmf_MasOrchestration.yaml` | Excluded from this read-only client; write/orchestration path with separate token and idempotency semantics |
| Namf Relay | `Nnwagent_Relay.md` | Excluded; standard Namf message relay/deployment behavior |

These contracts are pinned in
[the env-openamsm source snapshot](https://github.com/odango-chan/env-openamsm/tree/77c0a2f8511c3498d1270f059881b3cf0f7f5eeb/upstream/openamsm/tools/nw-agent/openapi/).
They remain owned by their respective NW Agent / OpenAMSM integration work.
Do not implement, wrap or proxy these endpoints as part of the initial CLI v1
slice. Any future expansion requires a separate capability-map entry and
acceptance against its own interface contract.
The only SMF SBI operations in the current CLI scope are the eight read-only
context-query routes documented in the interface inventory. Their OpenAMSM
internal route identifiers are private adapter details, not CA public names.
