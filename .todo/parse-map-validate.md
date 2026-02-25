ParseMap skips the wake-source validation that ParseINI performs.

internal/config/config.go ParseMap() should call validate(cfg) before
returning, same as ParseINI, so a Notehub-supplied config that strips
all wake sources is caught at parse time rather than silently booting
into a broken state.

Files: internal/config/config.go, internal/config/config_test.go

Add a test: ParseMap with no wake sources should return an error.
Run `make test` to verify.
