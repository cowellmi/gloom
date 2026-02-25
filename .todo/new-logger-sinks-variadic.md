NewLogger ignores its variadic sinks parameter — fix it to actually
register them.

internal/log/logger.go:
  func NewLogger(now time.Time, sinks ...sink.LogSink) *Logger {
      return &Logger{t: now}  // sinks is silently ignored
  }

Add a loop in NewLogger to call l.AddSink(s, config.LogLevelDebug)
for each sink, then return the logger. LogLevelDebug is used as the
default level for convenience-initialized sinks (most permissive).

Files: internal/log/logger.go, internal/log/logger_test.go

Run `make test` to verify.
