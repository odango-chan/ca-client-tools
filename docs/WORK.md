# Work

## Overall

Design: ACCEPTED for the OpenAMSM-only client boundary and initial SMF MAS
Query slice. Implementation: NOT_STARTED. Acceptance: NOT_RUN.
Roll-up: DEFERRED.

The repository is newly bootstrapped. The first implementation target is the
read-only SMF MAS QueryContext CLI. AMF/SMF metrics clients await exact
response-schema verification against the imported OpenAMSM source.

## Current scope matrix

| Scope | Design | Implementation | Acceptance | Tracking |
|---|---|---|---|---|
| OpenAMSM-only repository boundary | ACCEPTED | IMPLEMENTED (docs) | NOT_RUN | docs/architecture.md |
| SMF MAS Query client and CLI | ACCEPTED (v0.10 wire contract) | NOT_STARTED | NOT_RUN | docs/openamsm-interface-inventory.md |
| AMF `/ue-info` and `/gnb-info` clients | PROPOSED | NOT_STARTED | NOT_RUN | OpenAMSM response schema pending |
| SMF `/pdu-info` client | PROPOSED | NOT_STARTED | NOT_RUN | OpenAMSM response schema pending |
| UPF `/session-info` / exact N4 pair join | RETIRED for current OpenAMSM source | NOT_STARTED | NOT_APPLICABLE | Endpoint and pair absent |

## Blockers

- The OpenAMSM archive is not yet imported into
  `env-openamsm/upstream/openamsm`; static source findings are not build or
  live acceptance evidence.
- AMF/SMF metrics response schemas and pagination must be verified from the
  actual archive before their clients are implemented.

## Next actions

1. Implement and test the SMF MAS Query client/CLI against the frozen v0.10
   contract.
2. Verify the archive import/source status with env-openamsm.
3. Add AMF/SMF metrics clients after exact source-schema inspection.
4. Keep UPF session-info and N4 pair dependent features out of scope unless
   OpenAMSM gains a separately accepted interface.

## Authority / evidence links

- `env-openamsm/docs/openamsm-source-findings.md`
- SMF MAS QueryContext SBI v0.10 design specification
- `docs/architecture.md`
- `docs/openamsm-interface-inventory.md`
