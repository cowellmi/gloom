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

func LogMap(m map[string]any) {
	b := buf[:0]
	for k, v := range m {
		if s, ok := v.(string); ok {
			b = fmtbuf.Append(b, k)
			b = fmtbuf.Append(b, "=")
			b = fmtbuf.Append(b, s)
			b = fmtbuf.AppendByte(b, '\r')
			b = fmtbuf.AppendByte(b, '\n')
		}
	}
	W.Write(b)
}
