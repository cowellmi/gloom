# Verify `strings.SplitSeq` TinyGo compatibility

**severity:** low

## Location

`internal/config/config.go:90,296`

```go
for line := range strings.SplitSeq(string(data), "\n") { ... }
// and
for p := range strings.SplitSeq(value, ",") { ... }
```

## Problem

`strings.SplitSeq` returns an `iter.Seq[string]` iterator and was added to the Go standard library in Go 1.24. The `go test` path (host, standard Go toolchain) works fine if `go.mod` declares `go 1.24`. But TinyGo ships its own standard library shims, and TinyGo's support for Go 1.24 APIs (especially range-over-func iterators) may lag behind.

If TinyGo does not implement `strings.SplitSeq`, the embedded build will fail with a missing-symbol or unsupported-feature error, while `make test` continues to pass on the host — a silent build/test divergence.

## Steps

1. Check `go.mod` to confirm it declares `go 1.24` (or adjust if not).
2. Verify the TinyGo version in `flake.nix` supports `strings.SplitSeq`.
3. If TinyGo does not support it, replace with the portable equivalent:

```go
// Replace:
for line := range strings.SplitSeq(string(data), "\n") { ... }

// With:
for _, line := range strings.Split(string(data), "\n") { ... }
```

`strings.Split` has been in Go since 1.0 and is implemented in all TinyGo targets. The performance difference is negligible for config parsing (called once at boot). No allocations change — `strings.Split` allocates a `[]string`, which is the same as the host-side allocation budget since this is boot-time config parsing anyway.
