package fmtbuf_test

import (
	"math"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/fmtbuf"
)

func TestAppend2(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{5, "05"},
		{9, "09"},
		{10, "10"},
		{59, "59"},
	}
	for _, tt := range tests {
		var buf [4]byte
		got := string(fmtbuf.Append2(buf[:0], tt.n))
		if got != tt.want {
			t.Errorf("Append2(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestAppend4(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0000"},
		{5, "0005"},
		{2026, "2026"},
		{9999, "9999"},
	}
	for _, tt := range tests {
		var buf [4]byte
		got := string(fmtbuf.Append4(buf[:0], tt.n))
		if got != tt.want {
			t.Errorf("Append4(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestAppendTimestamp(t *testing.T) {
	ts := time.Date(2026, 2, 14, 9, 5, 7, 0, time.UTC)
	var buf [20]byte
	got := string(fmtbuf.AppendTimestamp(buf[:0], ts))
	want := "2026-02-14T09:05:07"
	if got != want {
		t.Errorf("AppendTimestamp = %q, want %q", got, want)
	}
}

func TestAppendSerialTimestamp(t *testing.T) {
	ts := time.Date(2026, 2, 14, 9, 5, 7, 0, time.UTC)
	var buf [16]byte
	got := string(fmtbuf.AppendSerialTimestamp(buf[:0], ts))
	want := "[09:05:07] "
	if got != want {
		t.Errorf("AppendSerialTimestamp = %q, want %q", got, want)
	}
}

// cap5 returns a []byte backed by a 5-byte array with cap=5.
func cap5() []byte {
	var arr [5]byte
	return arr[:0]
}

func TestAppend(t *testing.T) {
	tests := []struct {
		name string
		pre  string // bytes already in buffer before the call
		s    string
		want string
	}{
		{"empty buf empty s", "", "", ""},
		{"normal", "", "hi", "hi"},
		{"exact fit", "", "hello", "hello"},
		{"truncate by 1", "", "hello!", "hello"},
		{"truncate long", "", "hello world", "hello"},
		{"full buf unchanged", "hello", "x", "hello"},
		{"partial pre truncate", "hi", "xyz", "hixyz"},
		{"partial pre exact", "hi", "xyz", "hixyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := cap5()
			b = append(b, tt.pre...)
			b = fmtbuf.Append(b, tt.s)
			got := string(b)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if len(b) > cap(b) {
				t.Errorf("len %d > cap %d", len(b), cap(b))
			}
		})
	}
}

func TestAppendByte(t *testing.T) {
	b := cap5()
	b = fmtbuf.AppendByte(b, 'a')
	b = fmtbuf.AppendByte(b, 'b')
	b = fmtbuf.AppendByte(b, 'c')
	b = fmtbuf.AppendByte(b, 'd')
	b = fmtbuf.AppendByte(b, 'e')
	// buffer full — next call must be a no-op
	before := string(b)
	b = fmtbuf.AppendByte(b, 'f')
	if string(b) != before {
		t.Errorf("AppendByte on full buffer changed content: %q", string(b))
	}
	if string(b) != "abcde" {
		t.Errorf("got %q, want %q", string(b), "abcde")
	}
}

func TestAppendInt(t *testing.T) {
	tests := []struct {
		name string
		v    int64
		base int
		pre  string
		want string // expected content of full buffer
	}{
		{"zero", 0, 10, "", "0"},
		{"positive", 42, 10, "", "42"},
		{"negative", -7, 10, "", "-7"},
		{"hex", 255, 16, "", "ff"},
		{"truncate", 1234567890, 10, "", "12345"},
		{"min int64", math.MinInt64, 10, "", "-9223"},
		{"full buf noop", 99, 10, "hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := cap5()
			b = append(b, tt.pre...)
			b = fmtbuf.AppendInt(b, tt.v, tt.base)
			got := string(b)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if len(b) > cap(b) {
				t.Errorf("len %d > cap %d", len(b), cap(b))
			}
		})
	}
}

func TestAppendUint(t *testing.T) {
	tests := []struct {
		name string
		v    uint64
		base int
		pre  string
		want string
	}{
		{"zero", 0, 10, "", "0"},
		{"small", 7, 10, "", "7"},
		{"hex", 0xdeadbeef, 16, "", "deadb"},
		{"truncate decimal", 1234567890, 10, "", "12345"},
		{"full buf noop", 99, 10, "hello", "hello"},
		{"max uint64 hex", math.MaxUint64, 16, "", "fffff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := cap5()
			b = append(b, tt.pre...)
			b = fmtbuf.AppendUint(b, tt.v, tt.base)
			got := string(b)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if len(b) > cap(b) {
				t.Errorf("len %d > cap %d", len(b), cap(b))
			}
		})
	}
}

func TestAppendBytes(t *testing.T) {
	tests := []struct {
		name string
		pre  string
		s    []byte
		want string
	}{
		{"empty buf empty s", "", []byte{}, ""},
		{"normal", "", []byte("hi"), "hi"},
		{"exact fit", "", []byte("hello"), "hello"},
		{"truncate by 1", "", []byte("hello!"), "hello"},
		{"full buf unchanged", "hello", []byte("x"), "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := cap5()
			b = append(b, tt.pre...)
			b = fmtbuf.AppendBytes(b, tt.s)
			got := string(b)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if len(b) > cap(b) {
				t.Errorf("len %d > cap %d", len(b), cap(b))
			}
		})
	}
}

func TestNoAllocs(t *testing.T) {
	var arr [64]byte
	src := []byte("hello world truncated bytes")
	allocs := testing.AllocsPerRun(100, func() {
		b := arr[:0]
		b = fmtbuf.Append(b, "hello world truncated string")
		b = fmtbuf.AppendBytes(b, src)
		b = fmtbuf.AppendByte(b, '!')
		b = fmtbuf.AppendInt(b, -9876543210, 10)
		b = fmtbuf.AppendUint(b, 9876543210, 16)
		_ = b
	})
	if allocs > 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}
