# Low priority: deduplicate date-formatting helpers in internal/sink/file/file.go

## Context

`internal/sink/file/file.go` contains two sets of helpers that do the same thing — write a
zero-padded decimal integer into a buffer:

- `put2(b []byte, n int)` / `put4(b []byte, n int)` — write directly into a fixed-size `[]byte`
  slice at a given index (used only by `buildFilename`)
- `append2(buf []byte, n int) []byte` / `append4(buf []byte, n int) []byte` — append-style,
  return the grown slice (used by `appendTimestamp`)

`buildFilename` uses `put2`/`put4` via an intermediate `var date [8]byte` array:

```go
func buildFilename(spec FileSpec, t time.Time) string {
    y, m, d := t.Date()
    var date [8]byte
    put4(date[:], y)
    put2(date[4:], int(m))
    put2(date[6:], int(d))
    return spec.Dir + "/" + string(date[:]) + spec.Ext
}
```

## Fix

Rewrite `buildFilename` to use `append4`/`append2` with a stack-local buffer, then delete
`put2` and `put4`:

```go
func buildFilename(spec FileSpec, t time.Time) string {
    y, m, d := t.Date()
    var buf [8]byte
    b := append4(buf[:0], y)
    b = append2(b, int(m))
    b = append2(b, int(d))
    return spec.Dir + "/" + string(b) + spec.Ext
}
```

Remove `put2` and `put4` entirely.

## Impact

`internal/sink/file` only. No interface or API changes. Tests in `file_test.go` cover
`buildFilename` indirectly through `Record` filename checks.
