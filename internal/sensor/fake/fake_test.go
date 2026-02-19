package fake

import (
	"testing"
)

func TestDevice_Name(t *testing.T) {
	d := NewDevice()
	if got := d.Name(); got != "fake" {
		t.Errorf("Name() = %q, want %q", got, "fake")
	}
}

func TestDevice_Init(t *testing.T) {
	d := NewDevice()
	if err := d.Init(); err != nil {
		t.Errorf("Init() error: %v", err)
	}
}

func TestDevice_Measure(t *testing.T) {
	d := NewDevice()

	ms, err := d.Measure()
	if err != nil {
		t.Fatalf("Measure() error: %v", err)
	}

	if len(ms) != 1 {
		t.Fatalf("Measure() returned %d measurements, want 1", len(ms))
	}

	m := ms[0]
	if m.Label != "foo" {
		t.Errorf("Label = %q, want %q", m.Label, "foo")
	}
	if m.Unit != "bars" {
		t.Errorf("Unit = %q, want %q", m.Unit, "bars")
	}
	if m.Value == "" {
		t.Error("Value is empty, want a numeric string")
	}
}

func TestDevice_MeasureNoAlloc(t *testing.T) {
	d := NewDevice()

	ms1, _ := d.Measure()
	ms2, _ := d.Measure()

	if &ms1[0] != &ms2[0] {
		t.Error("Measure() allocated a new slice instead of reusing internal buffer")
	}
}
