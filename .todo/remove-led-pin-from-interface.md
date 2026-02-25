Remove Pin() from hal.LED — it is board topology, not LED behavior.

Pin() is only used in main.go to seed config.Default(board.LED.Pin(), ...).
The board already knows the pin because it created the LED with it.

Changes:
- hal/led.go: remove Pin() from the LED interface
- internal/led/led.go: remove the Pin() method
- cmd/gloom/board.go: add LEDPin hal.Pin field to Board struct
- cmd/gloom/board_feather-m0.go: set board.LEDPin = hal.Pin(machine.LED)
- cmd/gloom/main.go: replace board.LED.Pin() with board.LEDPin in
  config.Default() call and wherever else Pin() is used

Run `make test` to verify.
