# Marshal Config to Map

Similair to Config.Marshal, we should have a mehtod on config
that marshals its data to a map[string]any which will be used in
the `env.template` request in main. If `env.get` request returns 
empty response (indicating no env vars set), write default values
to `env.default` request body to set default values.
