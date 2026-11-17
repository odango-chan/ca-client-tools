// Package acceptance contains opt-in real-environment acceptance tests. The
// tests only build with the `acceptance` build tag and require a live OpenAMSM
// deployment plus compiled provider/CLI executables. Normal `go test ./...`
// runs do not compile or execute them.
package acceptance
