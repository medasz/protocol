//go:build windows

package stdio

import (
	"io"
	"os"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

type consoleWriter struct {
	handle   windows.Handle
	fallback io.Writer
}

func WrapConsoleWriter(w io.Writer) io.Writer {
	file, ok := w.(*os.File)
	if !ok {
		return w
	}

	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return w
	}
	return &consoleWriter{
		handle:   handle,
		fallback: w,
	}
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !utf8.Valid(p) {
		return w.fallback.Write(p)
	}

	utf16Data := utf16.Encode([]rune(string(p)))
	if len(utf16Data) == 0 {
		return len(p), nil
	}

	var written uint32
	if err := windows.WriteConsole(w.handle, &utf16Data[0], uint32(len(utf16Data)), &written, nil); err != nil {
		return w.fallback.Write(p)
	}
	return len(p), nil
}
