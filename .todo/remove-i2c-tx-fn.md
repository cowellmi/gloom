Board.I2C.TxFn is a redundant closure over Board.I2C.Bus — remove it.

TxFn exists only because notecard.New() takes tinynote.I2CTxFn from the
Blues SDK instead of hal.I2C. The closure just wraps Bus.Tx().

Changes:
- internal/notecard/notecard.go: change New() to accept hal.I2C instead
  of tinynote.I2CTxFn. Build the closure internally:
    d.ctx, err = tinynote.OpenI2C(addr, func(a uint16, w, r []byte) error {
        return bus.Tx(a, w, r)
    })
- cmd/gloom/board.go: remove TxFn from the I2C struct
- cmd/gloom/board_feather-m0.go: remove the TxFn closure setup
- cmd/gloom/main.go: pass board.I2C.Bus to notecard.New()

Run `make test` and `make` (tinygo build) to verify.
