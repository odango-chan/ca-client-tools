// Package acceptance contains opt-in real-environment acceptance tests. The
// tests only build with the `acceptance` build tag. The live typed tests require
// a live OpenAMSM deployment plus compiled production Provider/CLI executables;
// the generic fixture tests require only the providerfixture Provider binary and
// a deterministic loopback upstream started inside the test process.
// Normal `go test ./...` runs do not compile or execute them.
package acceptance
