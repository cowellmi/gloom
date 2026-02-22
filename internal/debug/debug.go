package debug

import "io"

var W io.Writer

var buf [128]byte

func Log(msg string) {
	b := buf[:0]
	b = append(b, msg...)
	b = append(b, '\r', '\n')
	W.Write(b)
}
