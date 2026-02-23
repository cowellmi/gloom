# No CSV header row in data files

**severity:** low

Data files are written as bare CSV (`timestamp,device,label,value,unit`) but there's no header row. Adding a header on file creation would make the CSVs self-documenting for researchers working with the data offline.

---

## Implementation

### File: `internal/sink/file/file.go`

In `openForDate`, after successfully opening `s.dataFile`, immediately write the header row. The header should be written unconditionally on every file open (initial open at boot and on daily rotation). A duplicate header on same-day reboot is acceptable — field devices rarely reboot mid-day.

Change this block (around line 186):
```go
if s.dataSpec.Dir != "" {
    f, err := s.open(buildFilename(s.dataSpec, t))
    if err != nil {
        errs = append(errs, err)
    } else {
        s.dataFile = f
    }
}
```

To:
```go
if s.dataSpec.Dir != "" {
    f, err := s.open(buildFilename(s.dataSpec, t))
    if err != nil {
        errs = append(errs, err)
    } else {
        s.dataFile = f
        if _, werr := s.dataFile.Write(csvHeader); werr != nil {
            s.dataFile = nil
            errs = append(errs, werr)
        }
    }
}
```

Add the header constant near the top of the file (after the `package` declaration imports):
```go
var csvHeader = []byte("timestamp,device,label,value,unit\n")
```

No changes needed to `Record`, `WriteLog`, `Flush`, or any other method.

---

### File: `internal/sink/file/file_test.go`

Several tests check exact file contents and must be updated to expect the header row as the first line.

**`TestRecord_WritesCSV`** — update `want`:
```go
want := "timestamp,device,label,value,unit\n2026-02-14T10:30:00,bme280,temp,22000,mC\n"
```

**`TestRecord_NegativeValue`** — update `want`:
```go
want := "timestamp,device,label,value,unit\n2026-02-14T10:30:00,ds18b20,temp,-5000,mC\n"
```

**`TestRotation_SameDateNoReopen`** — the file now has a header + 2 data rows = 3 lines. Update the assertion:
```go
if len(lines) != 3 {
    t.Errorf("expected 3 lines (header + 2 data rows), got %d", len(lines))
}
```
Also verify the first line is the header:
```go
if lines[0] != "timestamp,device,label,value,unit" {
    t.Errorf("first line = %q, want CSV header", lines[0])
}
```

**`TestRotation_OnDateChange`** — after rotation, `data/20260215.csv` is opened but no data is written to it in this test (only a log entry triggers rotation). The data file for day 2 will contain just the header. No assertion currently checks day2 data file contents, so no change needed there. However, day1 data file (`data/20260214.csv`) will now have header + 1 data row. If the test checks line count, update accordingly — currently it doesn't, so no change needed.

**All other tests** (`TestNew_OpensFilesForDate`, `TestFlush_SyncsOpenFiles`, `TestEmptyDir_SkipsFile`, `TestBuildFilename`) do not check CSV contents, so no changes needed.

---

## Validation

```
go test ./internal/sink/file/...
```

All tests should pass.
