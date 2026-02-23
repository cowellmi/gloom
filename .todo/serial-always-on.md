# Task: Make serial sink unconditional

## Goal

Wire the serial log and data sinks directly in `main.go` instead of through the
config system. Remove `"serial"` as a configurable sink name entirely. Only `"sd"`
remains as a configurable sink in `[device]`.

**Rationale:** UART0 and USB-CDC are always hardware-initialized at boot regardless
of config. Serial costs nothing extra during deep sleep (USB is detached anyway in
`samd21.go:detachUSB`). Removing serial from config simplifies both the code and
the INI file users need to write.

---

## Files to change

### 1. `internal/config/config.go`

**a) `Default()` (line 58)** — remove serial from both sink lists:
```go
// before
Device: Device{
    LogSinks: []LogSinkEntry{
        {Name: "serial", Level: log.LevelDebug},
    },
    DataSinks: []string{"serial"},
},

// after
Device: Device{
    LogSinks:  []LogSinkEntry{},
    DataSinks: []string{},
},
```

**b) `knownSinks` (line 215)** — remove `"serial"`:
```go
// before
var knownSinks = []string{"serial", "sd"}

// after
var knownSinks = []string{"sd"}
```

**c) `validateGroup` (line 202)** — remove the DataSinks check. With serial always
active, sensors always have somewhere to go. The full check was:
```go
if len(g.Sensors) > 0 && len(dev.DataSinks) == 0 {
    errs = append(errs, errors.New("["+g.Name+"] sensors require at least one data_sink in [device]"))
}
```
Delete those three lines entirely. The remaining check (interval or external_int_pin)
stays.

---

### 2. `cmd/gloom/main.go`

**a) Always wire serial to logger (around line 183)**

Before the `for _, ls := range cfg.Device.LogSinks` loop, add:
```go
logger.AddSink(serialSink, log.LevelDebug)
```

Then in the loop body, delete the `case "serial":` case entirely. The loop now
only handles `"sd"`. Result:
```go
logger.AddSink(serialSink, log.LevelDebug)

for _, ls := range cfg.Device.LogSinks {
    switch ls.Name {
    case "sd":
        if sdCardFileSink != nil {
            logger.AddSink(sdCardFileSink, ls.Level)
        } else {
            initWarns = append(initWarns, errors.New("log sink 'sd' configured but no SD card"))
        }
    }
}
```

**b) Always wire serial to recorders (around line 202)**

Before the `for _, name := range cfg.Device.DataSinks` loop, add:
```go
recorders = append(recorders, serialSink)
```

Then remove the `case "serial":` case from the loop. Result:
```go
var recorders []sensor.Recorder
recorders = append(recorders, serialSink)
for _, name := range cfg.Device.DataSinks {
    switch name {
    case "sd":
        if sdCardFileSink != nil {
            recorders = append(recorders, sdCardFileSink)
        }
    }
}
```

**c) Boot banner (around line 279)**

The existing log sinks / data sinks banner loops now only show SD if configured.
Add a static line before those loops to make serial visible:
```go
logger.Debug("serial: active")
```

Place it just before the `if len(cfg.Device.LogSinks) > 0` block.

---

### 3. `example.config.ini`

Rewrite to reflect that serial is implicit and only `sd` is configurable. Replace
the entire file with:
```ini
# --- Example ---
[device]
log_sinks = sd:info
data_sinks = sd

[weather]
interval = 1m
sensors = temperature, humidity
pulse_led = true

[rain]
external_int_pin = 7
sensors = tipping_bucket

[heartbeat]
interval = 1h
host = http://localhost:4000/heartbeat
payload = full


# --- Reference ---
#
# Serial (UART0 + USB-CDC) is always active at LevelDebug. No config needed.
#
# [device]
# log_sinks    = sink[:level], ...  (levels: debug [default], info, warn, error)
# data_sinks   = sink, ...          (sinks: sd)
#
# [group]
# interval         = duration        (Go syntax: 5s, 3m, 1h)
# external_int_pin = pin
# sensors          = name, ...
# pulse_led        = true|false
# host             = url
# payload          = none|min|full
#
#
# --- Feather M0 board sensors ---
#
# vbat  — LiPo/USB voltage via D9 (2:1 divider)
```

---

### 4. `internal/config/config_test.go`

**a) `TestDefault` (line 14)** — update expectations:
- `LogSinks` should be empty (len 0)
- `DataSinks` should be empty (len 0)

Replace the LogSinks and DataSinks checks:
```go
if len(cfg.Device.LogSinks) != 0 {
    t.Errorf("LogSinks = %v, want empty (serial is implicit)", cfg.Device.LogSinks)
}
if len(cfg.Device.DataSinks) != 0 {
    t.Errorf("DataSinks = %v, want empty (serial is implicit)", cfg.Device.DataSinks)
}
```

**b) `TestParse_DeviceSection` (line 47)** — `"serial"` is no longer a valid sink
name, so change input to only use `"sd"`:
```go
input := []byte(`
[device]
log_sinks = sd:error
data_sinks = sd
`)
```
Update assertions to expect 1 sink each (not 2), and check for `"sd"`.

**c) `TestParse_LogSinksDefaultLevel` (line 73)** — change `serial` to `sd` in
input:
```go
input := []byte(`
[device]
log_sinks = sd
`)
```

**d) `TestParse_LogSinksInvalidLevel` (line 89)** — change `serial:verbose` to
`sd:verbose` (just testing the level parse error, not the sink name).

**e) `TestParse_SensorsWithoutDeviceDataSinks` (line 378)** — delete this test
entirely. The validation it covers has been removed (sensors always have serial).

**f) `TestParse_FullExample` (line 542)** — update the input to remove `serial`:
```go
log_sinks = sd:error
data_sinks = sd
```
Update assertions: `LogSinks` len 1, `DataSinks` len 1.

**g) Any other test input with `data_sinks = serial` or `log_sinks = serial`** —
replace with `sd` or omit the line entirely. Grep the file for `"serial"` to
catch them all. Affected tests include `TestParse_MultipleGroups`,
`TestParse_RepeatedSection`, `TestParse_CommentsAndBlanks`,
`TestParse_InlineComments`.

---

### 5. `internal/config/marshal_test.go`

**`TestMarshal_RoundTrip` (line 12)** — remove `"serial"` from LogSinks and
DataSinks in the test config. Serial is no longer marshalled. Update to use only
`"sd"` sinks, and update the assertions accordingly (expect 1 LogSink, 1 DataSink).

---

## Validation

Run: `nix develop --command make test`

All tests should pass. No new tests needed — existing test coverage is adequate
once updated.

The build (`make`) should also succeed. The `sink/serial` package is still imported
and used — it's just no longer wired through config.

---

## What does NOT change

- `internal/sink/serial/serial.go` — no changes needed
- `cmd/gloom/board_feather-m0.go` — hardware init was already unconditional
- `internal/config/marshal.go` — already only marshals what's in the struct; with
  serial gone from Device, the `[device]` section will only emit `sd` lines when
  present. No logic changes needed.
- The `sink/serial` import in `main.go` — still needed
