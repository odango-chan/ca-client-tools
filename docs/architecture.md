# CA Client Tools architecture

## 1. Purpose

CA Client Tools provides typed local client operations for the interfaces
already exposed by the selected OpenAMSM environment.

It is a client integration layer, not a second 5GC implementation, not an
Agent, not a Runtime, and not an OpenAMSM environment manager.

## 2. Component boundaries

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
          +---- HTTP/SBI POST -> OpenAMSM SMF MAS QueryContext
```

The exact Runtime-to-Host dispatch seam remains governed by the paired
ca-agent / ca-runtime contracts. This repository does not invent a second
execution ingress.

The CLI is only the local process boundary. It does not replace the protocol
provided by OpenAMSM. In particular, SMF MAS QueryContext remains a remote
HTTP/SBI POST with the OpenAMSM query-token header.

## 3. Responsibilities

### This repository

- OpenAMSM endpoint clients.
- Request validation and response-envelope validation.
- Bounded HTTP transport.
- A stable CLI with machine-readable JSON output.
- Focused client tests using HTTP fixtures.

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
| SMF MAS Query (SC/PA/UPN/SSM/IAM/UPS/CA/SAA) | Agent through declared Tool/Capability | Yes, subject to Runtime admission |
| AMF `/ue-info` | Host context/default-tool integration | No |
| AMF `/gnb-info` | Host context/default-tool integration | No |
| SMF `/pdu-info` | Host context/default-tool integration | No |
| UPF `/session-info` | Not available in inspected OpenAMSM source | No |

The Host may use internal default tools without exposing them as Agent-callable
Tools. The CLI itself does not enforce this visibility policy.

The SMF MAS wire response echoes the actual lookup key. For an Agent-callable
Observation, the Host resolves the Skill's opaque `ueRef` to a trusted SUPI/IMSI
before invoking the CLI, verifies the echoed key against that exact lookup, and
redacts/replaces the raw SUPI/IMSI before returning the normalized result to the
Agent. Raw CLI stdout is private Host input, not the Agent-visible Observation
payload.

## 5. OpenAMSM-only source policy

OpenAMSM is the sole target. The repository must not maintain a parallel
Open5GS backend, compatibility package or patch overlay.

The current source inspection identifies:

- AMF `GET /ue-info` and `GET /gnb-info`;
- SMF `GET /pdu-info`;
- eight SMF MAS QueryContext SBI endpoints;
- no UPF `GET /session-info`.

OpenAMSM SMF `/pdu-info` does not provide the SMF/UPF N4 SEID pair supplied
by the old Open5GS-specific patch. UPN MAS `n4Seid` is a single UPF SEID and
must not be reinterpreted as that pair.

Therefore the old Open5GS-only SMF N4-pair and UPF session-info patch
dependencies are excluded. Exact UPF session joins and PDR/FAR/QER reads are
not part of this client set until OpenAMSM exposes an independently accepted
source contract.

## 6. Initial implementation slice

The first implementation slice is the read-only SMF MAS Query client because
its eight paths, request envelope, authentication header, response envelope,
and error semantics are specified by the already implemented OpenAMSM
contract.

The client must preserve:

- the eight exact MAS identifiers and paths;
- identity-plus-PSI or UE-IP query-key rules;
- `X-OpenAMSM-Query-Token` authentication;
- `AVAILABLE` versus `UNAVAILABLE` response semantics;
- `404 SESSION_NOT_FOUND`, `403 FORBIDDEN`, `503 SERVICE_UNAVAILABLE`,
  and ProblemDetails errors.

A successful `UNAVAILABLE` response is not a transport error and is not a
business negative.

AMF and SMF metrics clients follow after their exact OpenAMSM response shapes
and paging rules are checked against the imported source archive.

## 7. Source and acceptance status

The current endpoint inventory is based on static inspection notes in
`env-openamsm`. The source archive has not yet been committed under
`env-openamsm/upstream/openamsm`; import and qualification are tracked by
[env-openamsm#1](https://github.com/odango-chan/env-openamsm/issues/1). This
repository may use the documented wire contract for fixture-driven client
work, but it must not claim a successful OpenAMSM build or live query until
the actual environment is available and tested.
