# Default Config `cmd/defcon`

uses `config.MarshalINI` to save the default config to project root `default.config.ini`. This will be a nice reference in case someone overwrites default on Notehub. Actually we should probably generate a default config.ini for each target since the default depends on board. So maybe a `ini/feather-m0.config.ini`?