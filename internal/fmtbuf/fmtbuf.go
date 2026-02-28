// Package fmtbuf provides truncating append helpers for fixed-capacity
// byte slices. Every function respects cap(b): if the incoming data
// would exceed the remaining capacity it is silently truncated rather
// than allowed to escape to the heap via a grow-triggered allocation.
//
// The caller idiom is unchanged from a plain-append approach:
//
//	b := m.buf[:0]
//	b = fmtbuf.Append(b, "hello ")
//	b = fmtbuf.AppendInt(b, n, 10)
package fmtbuf

import (
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
)

// AppendBytes appends s to b, truncating s if b's capacity would be exceeded.
// Never allocates.
func AppendBytes(b []byte, s []byte) []byte {
	avail := cap(b) - len(b)
	if avail <= 0 {
		return b
	}
	if len(s) > avail {
		s = s[:avail]
	}
	return append(b, s...)
}

// Append appends s to b, truncating s if b's capacity would be exceeded.
// Never allocates.
func Append(b []byte, s string) []byte {
	avail := cap(b) - len(b)
	if avail <= 0 {
		return b
	}
	if len(s) > avail {
		s = s[:avail]
	}
	return append(b, s...)
}

// AppendByte appends c to b if capacity allows. Never allocates.
func AppendByte(b []byte, c byte) []byte {
	if len(b) >= cap(b) {
		return b
	}
	return append(b, c)
}

// AppendInt appends the string representation of v in the given base.
// Uses a 20-byte stack-local scratch buffer; never allocates.
func AppendInt(b []byte, v int64, base int) []byte {
	var tmp [20]byte
	digits := strconv.AppendInt(tmp[:0], v, base)
	avail := cap(b) - len(b)
	if avail <= 0 {
		return b
	}
	if len(digits) > avail {
		digits = digits[:avail]
	}
	return append(b, digits...)
}

// AppendUint appends the string representation of v in the given base.
// Uses a 20-byte stack-local scratch buffer; never allocates.
func AppendUint(b []byte, v uint64, base int) []byte {
	var tmp [20]byte
	digits := strconv.AppendUint(tmp[:0], v, base)
	avail := cap(b) - len(b)
	if avail <= 0 {
		return b
	}
	if len(digits) > avail {
		digits = digits[:avail]
	}
	return append(b, digits...)
}

func AppendLevel(b []byte, level config.LogLevel) []byte {
	switch level {
	case config.LogLevelDebug:
		return append(b, "DBG"...)
	case config.LogLevelInfo:
		return append(b, "INF"...)
	case config.LogLevelWarn:
		return append(b, "WRN"...)
	case config.LogLevelError:
		return append(b, "ERR"...)
	default:
		return append(b, "LVL"...)
	}
}

// Append2 appends a two-digit zero-padded decimal representation of n (0–99).
func Append2(buf []byte, n int) []byte {
	return append(buf, byte('0'+n/10), byte('0'+n%10))
}

// Append4 appends a four-digit zero-padded decimal representation of n (0–9999).
func Append4(buf []byte, n int) []byte {
	return append(buf,
		byte('0'+n/1000),
		byte('0'+(n/100)%10),
		byte('0'+(n/10)%10),
		byte('0'+n%10),
	)
}

// AppendTimestamp appends an ISO 8601 timestamp without timezone: YYYY-MM-DDTHH:MM:SS.
func AppendTimestamp(buf []byte, t time.Time) []byte {
	y, mon, d := t.Date()
	h, min, sec := t.Clock()
	buf = Append4(buf, y)
	buf = AppendByte(buf, '-')
	buf = Append2(buf, int(mon))
	buf = AppendByte(buf, '-')
	buf = Append2(buf, d)
	buf = AppendByte(buf, 'T')
	buf = Append2(buf, h)
	buf = AppendByte(buf, ':')
	buf = Append2(buf, min)
	buf = AppendByte(buf, ':')
	buf = Append2(buf, sec)
	return buf
}

// AppendSerialTimestamp appends a bracketed HH:MM:SS timestamp followed by a space: "[HH:MM:SS] ".
func AppendSerialTimestamp(buf []byte, t time.Time) []byte {
	buf = AppendByte(buf, '[')
	buf = Append2(buf, t.Hour())
	buf = AppendByte(buf, ':')
	buf = Append2(buf, t.Minute())
	buf = AppendByte(buf, ':')
	buf = Append2(buf, t.Second())
	buf = AppendByte(buf, ']')
	buf = AppendByte(buf, ' ')
	return buf
}

// AppendDuration appends a rounded human-readable duration (e.g. "5s", "3m", "2h", "1d").
// It rounds to the nearest whole unit rather than truncating.
func AppendDuration(b []byte, d time.Duration) []byte {
	switch {
	case d < time.Minute:
		secs := (d + time.Second/2) / time.Second
		b = AppendInt(b, int64(secs), 10)
		return AppendByte(b, 's')
	case d < time.Hour:
		mins := (d + time.Minute/2) / time.Minute
		b = AppendInt(b, int64(mins), 10)
		return AppendByte(b, 'm')
	case d < 24*time.Hour:
		hrs := (d + time.Hour/2) / time.Hour
		b = AppendInt(b, int64(hrs), 10)
		return AppendByte(b, 'h')
	default:
		days := (d + 12*time.Hour) / (24 * time.Hour)
		b = AppendInt(b, int64(days), 10)
		return AppendByte(b, 'd')
	}
}
