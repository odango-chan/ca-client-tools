package providerapp

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"sync"
)

// ProviderVersion is the resident Provider implementation version reported by
// initialize. It is independent of the CLI build version.
const ProviderVersion = "1.0.0"

var buildDigestOnce sync.Once
var buildDigestValue string

// BuildDigest returns a reproducible sha256 digest of the installed Provider
// executable. The Host independently verifies the executable/package digest
// against trusted deployment metadata; the value reported here is only a
// consistency check. If the executable cannot be read, a build-info digest is
// used instead so initialization still reports a well-formed value.
func BuildDigest() string {
	buildDigestOnce.Do(func() {
		buildDigestValue = executableDigest()
	})
	return buildDigestValue
}

func executableDigest() string {
	executable, err := os.Executable()
	if err == nil {
		if file, openErr := os.Open(executable); openErr == nil {
			hasher := sha256.New()
			if _, copyErr := copyBounded(hasher, file); copyErr == nil {
				_ = file.Close()
				return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
			}
			_ = file.Close()
		}
	}
	fallback := runtime.Version() + "|" + runtime.GOOS + "|" + runtime.GOARCH + "|" + ProviderVersion
	sum := sha256.Sum256([]byte(fallback))
	return "sha256:" + hex.EncodeToString(sum[:])
}
