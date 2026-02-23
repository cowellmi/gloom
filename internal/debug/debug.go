package debug

import (
	"io"

	"github.com/cowellmi/gloom/internal/fmtbuf"
)

var W io.Writer

var buf [128]byte

func Log(msg string) {
	b := buf[:0]
	b = fmtbuf.Append(b, msg)
	b = fmtbuf.AppendByte(b, '\r')
	b = fmtbuf.AppendByte(b, '\n')
	W.Write(b)
}
