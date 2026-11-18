# Host Artifact Store conformance fixtures v1

**Contract identity:** `ca-host.artifact-store/v1`
**Normative document:** [Host Artifact Store contract v1](../../docs/design/host-artifact-store-v1.md)
**JSON schemas:** [`schemas/artifact-store/v1/`](../../schemas/artifact-store/v1)

These fixtures are the shared, provider-neutral conformance vectors for the
Host Artifact Store contract. A consumer repository pins the exact bytes by
copying this directory and recording the `MANIFEST.sha256` digest before
enabling artifact streaming.

## Envelope

Every case file is one JSON object:

| Member | Meaning |
|---|---|
| `fixtureVersion` | exactly `ca-host.artifact-store/v1` |
| `case` | stable case identifier |
| `description` | human-readable purpose |
| `value` / `request` / `response` | the wire value under test |
| `expect` | expected classification (`valid`, `reason`, `providerFault`, `hostFault`, `published`) |

Placeholders such as `{artifactRef}`, `{processToken}`, `{invocationId}`,
`{digest}` and `{bytes}` describe substitutions; a fixture never contains a
real token, real Host URL or secret. Fixture bytes come from
`digest-vectors.json`; `base64` there is only a fixture encoding and is never a
transfer mechanism.

## Case inventory

- `reference-*.json` — the only approved reference JSON value and its invalid
  shapes (extra member, bad token, nested value, plain string/path).
- `channel-config-*.json` — bootstrap validation (loopback-only `baseUrl`,
  printable bounded token).
- `channel-get-input-*.json` — input resolution success and every stable
  channel error with the Provider fault it maps to.
- `channel-post-output-*.json` — output staging success, truncation, size,
  quota, duplicate slot and closed invocation; a staged reference is not
  published until the invocation result is accepted.
- `slots-binary.json`, `slots-multipart.json` — descriptor `artifactSlots`
  extension, including a nested array JSON Pointer for a multipart output.
- `arguments-refs-multipart.json` — nested request JSON plus a `payloads` map
  whose values are references.
- `result-refs-multipart.json`, `result-refs-foreign.json` — accepted result
  projection and rejection of a reference the Host did not stage.
- `digest-vectors.json` — exact byte vectors with size, SHA-256 and media type.

## Verification

`MANIFEST.sha256` lists the SHA-256 of every fixture file in this directory in
sorted order (`<hex>  <filename>`), so both sides can prove they consume the
same bytes. The Host contract test suite validates every case; the Provider
conformance suite validates the same bytes from its pinned copy.
