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

import "strconv"

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
