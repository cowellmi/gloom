package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// --- MarshalJSON ---

func TestMarshalJSON_Default(t *testing.T) {
	cfg := Default(hal.Pin(13), hal.Pin(5), []string{"vbat"}, []hal.Pin{11})
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"led_pin": 13`) {
		t.Errorf("expected led_pin 13:\n%s", s)
	}
	if !strings.Contains(s, `"rtc_int_pin": 5`) {
		t.Errorf("expected rtc_int_pin 5:\n%s", s)
	}
	if !strings.Contains(s, `"sd_chip_select_pins": [11]`) {
		t.Errorf("expected sd_chip_select_pins:\n%s", s)
	}
	if !strings.Contains(s, `"serial"`) {
		t.Errorf("expected serial sink:\n%s", s)
	}
	if !strings.Contains(s, `"blues"`) {
		t.Errorf("expected blues sink:\n%s", s)
	}
	if !strings.Contains(s, `"sample"`) {
		t.Errorf("expected sample group:\n%s", s)
	}
	if !strings.Contains(s, `"interval": "5s"`) {
		t.Errorf("expected interval 5s:\n%s", s)
	}
}

func TestMarshalJSON_NoPinOmitted(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "led_pin") {
		t.Errorf("led_pin should be omitted when NoPin:\n%s", s)
	}
	if strings.Contains(s, "rtc_int_pin") {
		t.Errorf("rtc_int_pin should be omitted when NoPin:\n%s", s)
	}
	if strings.Contains(s, "sd_chip_select_pins") {
		t.Errorf("sd_chip_select_pins should be omitted when empty:\n%s", s)
	}
}

func TestMarshalJSON_SinksFixedOrder(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	serialIdx := strings.Index(s, `"serial"`)
	bluesIdx := strings.Index(s, `"blues"`)
	sdIdx := strings.Index(s, `"sd"`)
	if serialIdx < 0 || bluesIdx < 0 || sdIdx < 0 {
		t.Fatalf("missing sinks in output:\n%s", s)
	}
	if !(serialIdx < bluesIdx && bluesIdx < sdIdx) {
		t.Errorf("sinks not in order serial<blues<sd: serial=%d blues=%d sd=%d", serialIdx, bluesIdx, sdIdx)
	}
}

func TestMarshalJSON_GroupsSortedAlphabetically(t *testing.T) {
	cfg := Config{
		Sinks: map[string]SinkConfig{
			"serial": {LogLevel: LogLevelDebug},
		},
		Groups: map[string]Group{
			"sample": {Interval: 5 * time.Second},
			"rain":   {InterruptPins: []hal.Pin{7}},
		},
	}
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	rainIdx := strings.Index(s, `"rain"`)
	sampleIdx := strings.Index(s, `"sample"`)
	if rainIdx < 0 || sampleIdx < 0 {
		t.Fatalf("missing groups:\n%s", s)
	}
	// "rain" < "sample" alphabetically
	if rainIdx > sampleIdx {
		t.Errorf("groups not in alphabetical order: rain=%d sample=%d\n%s", rainIdx, sampleIdx, s)
	}
}

func TestMarshalJSON_GroupFields(t *testing.T) {
	cfg := Config{
		Groups: map[string]Group{
			"sample": {
				Interval: time.Minute,
				Sensors:  []string{"temperature", "humidity"},
				PulseLED: true,
			},
			"rain": {
				InterruptPins: []hal.Pin{7},
				Sensors:       []string{"bucket"},
			},
		},
	}
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"interval": "1m"`) {
		t.Errorf("expected interval 1m:\n%s", s)
	}
	if !strings.Contains(s, `"temperature"`) {
		t.Errorf("expected temperature sensor:\n%s", s)
	}
	if !strings.Contains(s, `"pulse_led": true`) {
		t.Errorf("expected pulse_led true:\n%s", s)
	}
	if !strings.Contains(s, `"interrupt_pins": [7]`) {
		t.Errorf("expected interrupt_pins [7]:\n%s", s)
	}
}

func TestMarshalJSON_ZeroGroupFieldsOmitted(t *testing.T) {
	cfg := Config{
		Groups: map[string]Group{
			"s": {Interval: 5 * time.Second},
		},
	}
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	s := string(data)
	for _, absent := range []string{"sensors", "interrupt_pins", "pulse_led"} {
		if strings.Contains(s, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, s)
		}
	}
}

func TestMarshalJSON_DurationFormatting(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, `"5s"`},
		{time.Minute, `"1m"`},
		{2 * time.Hour, `"2h"`},
		{90 * time.Second, `"90s"`},
	}
	for _, tt := range tests {
		cfg := Config{
			Groups: map[string]Group{"s": {Interval: tt.d}},
		}
		data, err := cfg.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON() error for %v: %v", tt.d, err)
		}
		if !strings.Contains(string(data), tt.want) {
			t.Errorf("duration %v: want %s in output:\n%s", tt.d, tt.want, data)
		}
	}
}

func TestMarshalJSON_MultipleSDPins(t *testing.T) {
	cfg := Config{
		SDChipSelectPins: []hal.Pin{11, 10},
		Groups:           map[string]Group{"s": {Interval: 5 * time.Second}},
	}
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}
	if !strings.Contains(string(data), "11, 10") {
		t.Errorf("expected 11, 10 in sd_chip_select_pins:\n%s", data)
	}
}

// --- MarshalMap ---

func TestMarshalMap_RoundTrip(t *testing.T) {
	orig := Config{
		LEDPin:           hal.Pin(13),
		SDChipSelectPins: []hal.Pin{11},
		Sinks: map[string]SinkConfig{
			"serial": {LogLevel: LogLevelError},
			"blues":  {LogLevel: LogLevelWarn},
			"sd":     {LogLevel: LogLevelDebug},
		},
		Groups: map[string]Group{
			"sample": {Interval: time.Minute, Sensors: []string{"vbat"}, PulseLED: true},
			"rain":   {InterruptPins: []hal.Pin{7}, Sensors: []string{"bucket"}},
		},
	}

	m := orig.MarshalMap()
	got := testDefault()
	if err := ParseMap(&got, m); err != nil {
		t.Fatalf("ParseMap(MarshalMap()) error: %v", err)
	}

	if got.LEDPin != hal.Pin(13) {
		t.Errorf("LEDPin = %d, want 13", got.LEDPin)
	}
	if got.Sinks["serial"].LogLevel != LogLevelError {
		t.Errorf("serial log level = %v, want error", got.Sinks["serial"].LogLevel)
	}
	if len(got.Groups) != 2 {
		t.Fatalf("Groups len = %d, want 2", len(got.Groups))
	}
	sample := got.Groups["sample"]
	if sample.Interval != time.Minute {
		t.Errorf("sample.Interval = %v, want 1m", sample.Interval)
	}
	if !sample.PulseLED {
		t.Error("sample.PulseLED = false, want true")
	}
	rain := got.Groups["rain"]
	if len(rain.InterruptPins) != 1 || rain.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("rain.InterruptPins = %v, want [7]", rain.InterruptPins)
	}
}

func TestMarshalMap_SinksAlwaysEmitted(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	m := cfg.MarshalMap()

	sinks, ok := m["sinks"].(map[string]interface{})
	if !ok {
		t.Fatalf("sinks should be map[string]interface{}, got %T", m["sinks"])
	}
	for _, name := range []string{"serial", "blues", "sd"} {
		if _, ok := sinks[name]; !ok {
			t.Errorf("sinks missing %q", name)
		}
	}
}

func TestMarshalMap_SinkLogLevel(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	m := cfg.MarshalMap()

	sinks := m["sinks"].(map[string]interface{})
	blues := sinks["blues"].(map[string]interface{})
	if blues["log_level"] != "off" {
		t.Errorf("blues log_level = %v, want off", blues["log_level"])
	}
}

func TestMarshalMap_LEDPinNone(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	m := cfg.MarshalMap()

	if m["led_pin"] != "none" {
		t.Errorf("led_pin = %v, want none", m["led_pin"])
	}
}

func TestMarshalMap_EmptySlicesOmitted(t *testing.T) {
	cfg := Default(hal.NoPin, hal.NoPin, nil, nil)
	m := cfg.MarshalMap()

	if _, ok := m["sd_chip_select_pins"]; ok {
		t.Errorf("sd_chip_select_pins should be absent when empty, got %v", m["sd_chip_select_pins"])
	}
	if _, ok := m["rtc_int_pin"]; ok {
		t.Errorf("rtc_int_pin should be absent when NoPin, got %v", m["rtc_int_pin"])
	}
}

func TestMarshalMap_GroupsObject(t *testing.T) {
	cfg := Config{
		Sinks: map[string]SinkConfig{"serial": {LogLevel: LogLevelDebug}},
		Groups: map[string]Group{
			"sample": {Interval: 5 * time.Second, Sensors: []string{"vbat"}, PulseLED: true},
		},
	}
	m := cfg.MarshalMap()

	groups, ok := m["groups"].(map[string]interface{})
	if !ok {
		t.Fatalf("groups should be map[string]interface{}, got %T", m["groups"])
	}
	gmap, ok := groups["sample"].(map[string]interface{})
	if !ok {
		t.Fatalf("sample should be map[string]interface{}, got %T", groups["sample"])
	}
	if gmap["interval"] != "5s" {
		t.Errorf("interval = %v, want 5s", gmap["interval"])
	}
	if gmap["sensors"] != "vbat" {
		t.Errorf("sensors = %v, want vbat", gmap["sensors"])
	}
	if gmap["pulse_led"] != "true" {
		t.Errorf("pulse_led = %v, want true", gmap["pulse_led"])
	}
}

func TestMarshalMap_GroupPulseLEDFalseOmitted(t *testing.T) {
	cfg := Config{
		Groups: map[string]Group{"s": {Interval: 5 * time.Second}},
	}
	m := cfg.MarshalMap()
	groups := m["groups"].(map[string]interface{})
	gmap := groups["s"].(map[string]interface{})

	if _, ok := gmap["pulse_led"]; ok {
		t.Errorf("pulse_led should be absent when false, got %v", gmap["pulse_led"])
	}
}
