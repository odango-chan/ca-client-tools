# OpenAMSM read-only end-to-end acceptance

Status: **NOT RUN — no reachable deployment origin/port or query token has been supplied**

This plan validates the existing OpenAMSM implementation without changing the
server. It covers the eight private SMF MAS Context Query routes first.
AMF/SMF metrics list commands are supplementary diagnostics, not prerequisites
for the primary SMF query acceptance.

## 1. Trusted deployment configuration

Inject these values through the Host/deployment environment. Do not put the
query token in command arguments, request JSON, source control or test logs.

- `CA_OPENAMSM_SMF_SBI_ORIGIN`: existing SMF SBI server origin, for example
  `http://<smf-host>:<port>`.
- `CA_OPENAMSM_QUERY_TOKEN`: the existing OpenAMSM query token.

The generic SBI transport in PR #2 is h2c-only (HTTP/2 prior knowledge), but
the existing typed SMF MAS adapter has not yet been migrated to it. Until that
gated migration is complete, do not claim that the current MAS CLI calls have
passed h2c negotiation. The live query acceptance below validates the existing
OpenAMSM API and response semantics; h2c integration is a separate transport
gate. No custom TLS/OAuth client or server patch is required.

## 2. Primary scenario: all eight SMF Context Query routes

Use the known test lookup:

~~~json
{
  "supi": "imsi-001010000000001",
  "pduSessionId": 5
}
~~~

The test values are SUPI `imsi-001010000000001` and PSI `5`. The same
lookup is sent to each public CA context type; the adapter maps each public
type to the existing OpenAMSM MAS route.

~~~sh
for type in session policy up-node signaling-session ip-address up-selection charging session-auth; do
  printf '%s\n' '{"supi":"imsi-001010000000001","pduSessionId":5}' |
    ca-client-tools smf context query --type "$type"
done
~~~

For each of the eight calls, verify:

1. The request reaches the existing SMF SBI listener at the operation path
   corresponding to the selected public type.
2. The configured `X-OpenAMSM-Query-Token` is sent by trusted configuration.
3. The response is HTTP 200 with JSON and a normal `data` envelope.
4. `data.type` is the public CA type; OpenAMSM's internal `masId` is not
   exposed.
5. `data.key` echoes the normalized SUPI and PDU Session ID.
6. `data.status` is either `AVAILABLE` or `UNAVAILABLE`. Both are valid
   query outcomes. An `UNAVAILABLE` result must have an empty `context`;
   it is not a transport or CLI failure.
7. Record each type's status without requiring all eight context providers to
   return `AVAILABLE`.
8. Record the protocol actually negotiated. h2c is a separate acceptance gate
   until the MAS adapter is migrated to the shared transport.

Do not require an AMF or SMF metrics list query before these calls. The
identity-plus-PSI lookup is the primary acceptance path. IPv4/IPv6 keys may be
used as a separate supplementary test when the deployment has a known,
consistent UE address.

## 3. Error and negative-path checks

- Unknown context type: rejected locally as invalid input; no network request.
- Missing query key or invalid PSI: rejected locally; no network request.
- HTTP 403: query-token/deployment authorization failure.
- HTTP 404 with `SESSION_NOT_FOUND`: preserve as a remote error; do not
  reinterpret it as `UNAVAILABLE`.
- HTTP 200 with `status: UNAVAILABLE` and `context: {}`: successful query.
- Malformed JSON, mismatched `masId`, malformed echoed key or non-empty
  UNAVAILABLE context: protocol failure.
- Cancellation: no partial stdout result and no secret/token in stderr.

## 4. Supplementary metrics checks (not a prerequisite)

When AMF/SMF metrics origins are available, optionally run:

~~~sh
ca-client-tools amf ue list --page 0 --page-size 10
ca-client-tools amf gnb list --page 0 --page-size 10
ca-client-tools smf pdu list --page 0 --page-size 10
~~~

Verify the source pager values and item counts. These are diagnostic
projections and do not establish an atomic cross-NF snapshot.

## 5. Acceptance record

Record the tested repository commit, Go version, OpenAMSM source revision,
whether h2c was negotiated, the eight public types and their returned status,
and any HTTP error cause. Redact the service host if required by deployment
policy. Never record the query token or forward raw subscriber context to an
Agent.

No live origin/port or query token is currently available in this workstream,
so this plan is ready but **live acceptance remains NOT RUN**.
