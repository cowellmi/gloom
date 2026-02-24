package sink

import (
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/sensor"
)

// CSVRecorder implements sensor.Recorder, writing one CSV row per
// Reading to an io.Writer:
//
//	timestamp,id,label,value,unit
type CSVRecorder struct {
	w   io.Writer
	buf [256]byte
}

// NewCSVRecorder wraps w as a CSVRecorder.
func NewCSVRecorder(w io.Writer) *CSVRecorder {
	return &CSVRecorder{w: w}
}

func (r *CSVRecorder) Record(t time.Time, id string, readings []sensor.Reading) error {
	for _, rd := range readings {
		b := r.buf[:0]
		b = appendTimestamp(b, t)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, id)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, rd.Label)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.AppendInt(b, int64(rd.Value), 10)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, rd.Unit)
		b = fmtbuf.AppendByte(b, '\n')
		if _, err := r.w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

func (r *CSVRecorder) Flush() error { return nil }

// --- shared timestamp helpers (YYYY-MM-DDTHH:MM:SS) ---

func appendTimestamp(buf []byte, t time.Time) []byte {
	y, mon, d := t.Date()
	h, min, sec := t.Clock()
	buf = append4(buf, y)
	buf = fmtbuf.AppendByte(buf, '-')
	buf = append2(buf, int(mon))
	buf = fmtbuf.AppendByte(buf, '-')
	buf = append2(buf, d)
	buf = fmtbuf.AppendByte(buf, 'T')
	buf = append2(buf, h)
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, min)
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, sec)
	return buf
}

func append2(buf []byte, n int) []byte {
	return append(buf, byte('0'+n/10), byte('0'+n%10))
}

func append4(buf []byte, n int) []byte {
	return append(buf,
		byte('0'+n/1000),
		byte('0'+(n/100)%10),
		byte('0'+(n/10)%10),
		byte('0'+n%10),
	)
}
