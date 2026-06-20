//go:build !baremetal || tkey

package debug

import (
	"io"
	"os"
)

func defaultOutput() io.Writer {
	return os.Stderr
}

func Ready() bool {
	return true
}
