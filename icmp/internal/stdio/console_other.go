//go:build !windows

package stdio

import "io"

func WrapConsoleWriter(w io.Writer) io.Writer {
	return w
}
