# Free5GC upstream source audit — initial inventory

**Audit date:** 2026-11-24  
**Status:** SOURCE INVENTORY / NOT QUALIFIED  
**Purpose:** Identify whether the pinned Free5GC modules are a sound base for CA's
NAS, NGAP and standalone N2 Transfer integration. This is a source-level
inventory, not a per-IE conformance result or a Release 19 qualification.

## 1. Pinned source baseline

| Module | Version | Commit | Declared Go |
| --- | --- | --- | --- |
| github.com/free5gc/ngap | v1.2.0 | 7c6125004f5fa914412a4a87de3d48f0ca2ab1c8 | 1.26.2 |
| github.com/free5gc/nas | v1.3.1 | ff5ea67383b7b5841247aab712889890b4c67d55 | 1.26.2 |

Both repositories carry Apache-2.0 licenses. The versions match the current
CA Go toolchain, but this is only a build-compatibility observation.

Sources:
- [NGAP go.mod](https://github.com/free5gc/ngap/blob/v1.2.0/go.mod)
- [NAS go.mod](https://github.com/free5gc/nas/blob/v1.3.1/go.mod)
- [NGAP LICENSE](https://github.com/free5gc/ngap/blob/v1.2.0/LICENSE)
- [NAS LICENSE](https://github.com/free5gc/nas/blob/v1.3.1/LICENSE)

## 2. NAS 5GSM inventory

The pinned NAS source has an explicit 20-branch 5GSM dispatch in
message.ParseGSM. The 20 branches cover the 5GSM PDU Session message family,
5GSM Status, Service-Level Authentication and Remote UE Report messages.

The package tree contains:

| Area | Implementation files | Test files |
| --- | ---: | ---: |
| message package, all NAS message families | 62 | 45 |
| IE package | 152 | 87 |

These are file counts, not test-coverage percentages. The test counts include
5GMM and 5GSM and do not prove every message / IE combination is tested.

### 2.1 Important decode behavior

ParseGSM chooses a concrete message type from the message type octet, then
calls that message's UnmarshalBinary.

If the message decoder reports recognized-but-unimplemented optional IEs,
ParseGSM returns the partially populated message together with the
IEToDoList error. Callers must not discard that error or treat the returned
message as a complete decode.

A source search at the pinned commit found IEToDoList handling in all 15
PDU Session message implementation files:

- PDU Session Authentication Command / Complete / Result;
- PDU Session Establishment Request / Accept / Reject;
- PDU Session Modification Request / Reject / Command / Complete /
  Command Reject;
- PDU Session Release Request / Reject / Command / Complete.

This does not mean every packet of those types fails. It means each decoder has
at least one recognized IE path that can be reported as not implemented. The
exact IEI, direction, conditionality and effect on the selected OpenAMSM flows
must be itemized before the facade advertises support.

Unknown IEIs whose length / framing cannot be determined fail decoding rather
than being silently skipped.

Sources:
- [NAS ParseGSM](https://github.com/free5gc/nas/blob/v1.3.1/message/message.go#L614-L675)
- [Establishment Request decoder](https://github.com/free5gc/nas/blob/v1.3.1/message/pdu_session_establishment_request.go#L74-L120)
- [Modification Request decoder](https://github.com/free5gc/nas/blob/v1.3.1/message/pdu_session_modification_request.go)
- [Release Request decoder](https://github.com/free5gc/nas/blob/v1.3.1/message/pdu_session_release_request.go)

## 3. NGAP inventory

The pinned NGAP source has 120 explicit message dispatch targets across the
InitiatingMessage, SuccessfulOutcome and UnsuccessfulOutcome branches of
message.ParseMessageType.

The package tree contains:

| Area | Implementation files | Test files |
| --- | ---: | ---: |
| message package | 122 | 38 |
| ie package | 951 | 19 |

These counts describe source files only. They do not measure line coverage,
message conformance or IE-level Encode / Decode parity.

Sources:
- [NGAP message dispatch](https://github.com/free5gc/ngap/blob/v1.2.0/message/ngap_message.go)
- [NGAP package tree at pinned commit](https://github.com/free5gc/ngap/tree/v1.2.0)

## 4. Standalone PDU Session Resource Transfer inventory

The pinned NGAP IE package contains 13 standalone PDU Session Resource Transfer
root types:

| Transfer root | Dedicated test file |
| --- | --- |
| PDUSessionResourceModifyConfirmTransfer | No |
| PDUSessionResourceModifyIndicationTransfer | No |
| PDUSessionResourceModifyIndicationUnsuccessfulTransfer | No |
| PDUSessionResourceModifyRequestTransfer | Yes |
| PDUSessionResourceModifyResponseTransfer | Yes |
| PDUSessionResourceModifyUnsuccessfulTransfer | No |
| PDUSessionResourceNotifyReleasedTransfer | No |
| PDUSessionResourceNotifyTransfer | No |
| PDUSessionResourceReleaseCommandTransfer | Yes |
| PDUSessionResourceReleaseResponseTransfer | Yes |
| PDUSessionResourceSetupRequestTransfer | Yes |
| PDUSessionResourceSetupResponseTransfer | Yes |
| PDUSessionResourceSetupUnsuccessfulTransfer | No |

Six of the thirteen roots have dedicated test files. The six tested roots cover
basic marshal / unmarshal examples; this does not prove all nested IE choices,
optional fields, extension containers, unknown IE behavior or multi-QoS-flow
cases.

The current Mock AMF's Setup / Modify / Release business projections must be
mapped to these upstream types and tested with independent vectors.

Sources:
- [NGAP N2 Transfer source files](https://github.com/free5gc/ngap/tree/v1.2.0/ie)
- [Setup Request Transfer tests](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_setup_request_transfer_test.go)
- [Modify Request Transfer tests](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_modify_request_transfer_test.go)
- [Release Command Transfer tests](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_release_command_transfer_test.go)

## 5. NGAP APER risks confirmed in source

### 5.1 Extensible SEQUENCE is not implemented

ReadSequencePreambleBitMap and WriteSequencePreambleBitMap contain TODO
comments for extensible SEQUENCE support. The implementation treats the
extension bit as absent: the decoder reads the bit but discards its value,
and the encoder always writes false.

This is directly relevant to standalone N2 Transfer: the Setup Request,
Modify Request and Release Command Transfer root implementations call the
sequence preamble methods with valueExtensible=true.

A transfer whose extension bit is set therefore cannot be assumed to decode or
encode correctly with this pinned APER implementation.

Sources:
- [APER ReadSequencePreambleBitMap](https://github.com/free5gc/ngap/blob/v1.2.0/aper/unmarshal.go#L1087-L1104)
- [APER WriteSequencePreambleBitMap](https://github.com/free5gc/ngap/blob/v1.2.0/aper/marshal.go#L1036-L1048)
- [Setup Request Transfer](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_setup_request_transfer.go)
- [Modify Request Transfer](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_modify_request_transfer.go)
- [Release Command Transfer](https://github.com/free5gc/ngap/blob/v1.2.0/ie/pdu_session_resource_release_command_transfer.go)

### 5.2 Large SEQUENCE OF fragmentation is not implemented

The APER Read / Write implementation explicitly marks fragmentation unsupported
for large SEQUENCE OF values where length interleaves with elements. The
practical impact depends on the actual list bounds and deployment vectors;
it must be checked for each selected N2 / NGAP structure.

Sources:
- [APER unmarshal fragmentation path](https://github.com/free5gc/ngap/blob/v1.2.0/aper/unmarshal.go#L964-L1027)
- [APER marshal fragmentation path](https://github.com/free5gc/ngap/blob/v1.2.0/aper/marshal.go#L930-L965)

## 6. Unknown IE and exact-consumption behavior

The NGAP unknown-IE helper reads the IE criticality and an open-type value.
The open-type bytes are discarded by the helper. In the inspected
InitialUEMessage decoder, unknown IEs with Reject criticality cause an
Abstract Syntax Error; Ignore / Notify unknown IEs are skipped. Their original
value is not retained for a lossless decode / re-encode cycle.

This is protocol-aware skip behavior, not unknown-IE preservation.

The top-level message.Parse path calls ParseMessageType and then the
selected message's UnmarshalBinary. The inspected message decoder creates a
nested PerBitData for the open type and returns after known / unknown IE
processing; it does not expose the number of bytes consumed to the caller.

The standalone ie.UnmarshalBinary helper similarly constructs a
PerBitData, calls Read and returns the error without checking the final
byte / bit offset. aper.PerBitData does expose ByteOffset() and
BitOffset(), so a facade can implement a strict-consumption wrapper for
standalone IE types that expose Read. The full NGAP PDU path needs a separate
verified strategy; do not claim strict trailing-octet rejection until it is
tested.

Sources:
- [Unknown IE helper](https://github.com/free5gc/ngap/blob/v1.2.0/ie/ngap_ie.go#L26-L45)
- [InitialUEMessage unknown IE handling](https://github.com/free5gc/ngap/blob/v1.2.0/message/initial_ue_message.go#L724-L760)
- [NGAP Parse / ParseMessageType](https://github.com/free5gc/ngap/blob/v1.2.0/message/ngap_message.go#L21-L390)
- [PerBitData offsets](https://github.com/free5gc/ngap/blob/v1.2.0/aper/aper.go#L1-L40)

## 7. Consequence for CA

The pinned modules are a strong reuse candidate, but the source inventory does
not support a claim of complete NAS / NGAP Release 19 coverage.

Before integration is qualified:

1. Build a per-message / per-IE matrix for the exact Create, Modify and Release
   inputs / outputs exchanged by the current OpenAMSM and Mock AMF.
2. Resolve every NAS IEToDoList IE that occurs in those vectors.
3. Prove APER extension behavior for every selected N2 root and nested
   extensible structure.
4. Prove unknown IE criticality behavior and explicitly decide whether
   unknown values must be preserved or may be skipped.
5. Prove exact input consumption for complete NGAP PDU and standalone N2
   Transfer.
6. Add independent standard-derived vectors, malformed-input tests, round-trip
   tests and live interoperability evidence.

If a required case is blocked by upstream APER behavior, do not reimplement the
same parser independently in CA consumers. First evaluate an upstream fix or
a narrowly pinned fork; if that is not viable, compare a maintained alternative
or standards-derived generator.

**Current verdict: REUSE CANDIDATE / SOURCE AUDIT PARTIAL / NOT QUALIFIED.**