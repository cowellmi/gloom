package fmtbuf_test

import (
	"math"
	"testing"

	"github.com/cowellmi/gloom/internal/fmtbuf"
)

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
