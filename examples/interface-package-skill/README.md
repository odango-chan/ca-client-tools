# Example CA-SL skill linked against the published Interface Package

This directory is the release acceptance input for the generated Interface
Package. `ca-client-tools` publishes the Interface Package; this Skill is a
consumer fixture and is **not** part of the Provider or the Host.

- `skills/inspect-smf-session/` is a CA-SL `connectionagent.io/v1alpha5` Skill
  that references the published `openamsm.smf.mas.sc.query` Observation Tool and
  the `openamsm.smf.mas.context.read` Capability from the Interface Package.
- `internal/interfacepkg/compiler_link_test.go` builds the Development
  `BuildRequest` from the generated package and this Skill, then runs the real
  `ca-skill-compiler v2.1.0` binary (`CA_SKILLC`).

The expected result is `VALIDATED / LINKED / BUILT / NOT_ADMITTED`. `BUILT` is
not a Host admission and not a production qualification: a compiled Skill still
needs Host admission, Capability authorization, execution identity and Evidence.

Run the link gate locally:

```bash
curl -sSLo skillc https://github.com/odango-chan/ca-skill-compiler/releases/download/v2.1.0/skillc-v2.1.0-linux-amd64
chmod +x skillc
CA_SKILLC=$PWD/skillc go test ./internal/interfacepkg -run TestCompilerLinksGeneratedInterfacePackage -v
```
