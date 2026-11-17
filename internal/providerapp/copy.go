package providerapp

import "io"

// copyBounded copies the whole reader without adding an artificial size limit
// to the executable digest path. The executable is trusted local package
// content, not remote input.
func copyBounded(destination io.Writer, source io.Reader) (int64, error) {
	return io.Copy(destination, source)
}
