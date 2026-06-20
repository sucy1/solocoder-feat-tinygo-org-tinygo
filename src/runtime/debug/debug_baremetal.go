//go:build baremetal && !tkey

package debug

import (
	"io"
	"machine"
)

func Ready() bool {
	type hasDTR interface {
		DTR() bool
	}
	if ser, ok := machine.Serial.(hasDTR); ok {
		return ser.DTR()
	}
	return true
}

func defaultOutput() io.Writer {
	return machine.Serial
}
