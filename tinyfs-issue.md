# fatfs: default ffconf.h costs ~60 KB of flash on LFN/codepage tables most embedded targets don't need

## Summary

The vendored `ffconf.h` ships with `FF_USE_LFN = 2` and `FF_CODE_PAGE = 932` (Japanese DBCS). On constrained targets this puts ~60 KB of Unicode/codepage rodata into flash — over a third of the available space on a 256 KB MCU — for a feature many embedded projects never use.

Changing to `FF_USE_LFN = 0` and `FF_CODE_PAGE = 437` drops `tinyfs/fatfs` flash usage by roughly 60 KB with no loss of functionality for projects using 8.3 filenames.

## Reproducing

Save the following as `main.go`:

```go
package main

import (
	"machine"

	"tinygo.org/x/drivers/sdcard"
	"tinygo.org/x/tinyfs/fatfs"
)

func main() {
	dev := sdcard.New(machine.SPI0, machine.SPI0_SCK_PIN, machine.SPI0_SDO_PIN, machine.SPI0_SDI_PIN, machine.D10)
	if err := dev.Configure(); err != nil {
		return
	}

	fs := fatfs.New(&dev)
	fs.Configure(&fatfs.Config{SectorSize: 512})
	if err := fs.Mount(); err != nil {
		return
	}
}
```

Build with size output on any flash-constrained target (e.g. feather-m0, 256 KB flash):

```sh
tinygo build -size=full -target=feather-m0 -o=test.bin .
```

Note the `tinygo.org/x/tinyfs/fatfs` row — the majority of its flash usage is rodata (Unicode/codepage tables for LFN support).

Now vendor the dependency and apply the two-line fix:

```sh
go mod vendor
```

In `vendor/tinygo.org/x/tinyfs/fatfs/ffconf.h`, change:

```c
// Before:
#define FF_CODE_PAGE    932
#define FF_USE_LFN      2

// After:
#define FF_CODE_PAGE    437
#define FF_USE_LFN      0
```

Rebuild and compare the `tinyfs/fatfs` row:

```sh
tinygo build -size=full -target=feather-m0 -o=test.bin .
```

You can also compare using a fork with the fix already applied:

```sh
go get github.com/cowellmi/tinyfs@no-lfn
```

## Why the defaults hurt

- `FF_CODE_PAGE = 932` selects Japanese Shift-JIS, the largest DBCS codepage. Most TinyGo projects targeting Western-language environments need only codepage 437 (US) or at most 850 (Latin-1).
- `FF_USE_LFN = 2` enables long filename support with stack-allocated working buffers. Many embedded projects only use 8.3 filenames (e.g. `config.ini`, `20260214.csv`), so the LFN tables are pure overhead.
- The primary audience of `tinygo.org/x/tinyfs` is embedded developers on flash-constrained MCUs — the current defaults are optimized for the wrong use case.

## Suggested fix

Any of these would work — happy to submit a PR for whichever approach the maintainers prefer:

**Option A: Change the defaults (simplest)**

Set `FF_USE_LFN = 0` and `FF_CODE_PAGE = 437` in `ffconf.h`. Projects that need LFN can vendor and re-enable it. This matches the FatFs upstream default (`FF_USE_LFN = 0`).

**Option B: Build tags (no breakage)**

Keep current defaults for backward compatibility. Add a build tag (e.g. `tinyfs_no_lfn`) that selects an alternate `ffconf.h` with LFN disabled and a minimal codepage. Projects opt in to the smaller config at build time.

**Option C: Just fix the codepage (low-risk first step)**

Change `FF_CODE_PAGE` from 932 to 437. This is a safe change — 437 is the FatFs upstream default and covers ASCII filenames that virtually all TinyGo projects use. Address LFN separately.

## Environment

- TinyGo 0.40.1
- tinygo.org/x/tinyfs v0.5.0
- Target: feather-m0 (ATSAMD21, 256 KB flash)
