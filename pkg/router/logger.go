package router

import "log"

// Logger is the interface for logging within the router.
// Implementations must be safe for concurrent use.
type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Error(msg string, args ...any)
}

// defaultLogger writes to the standard log package with a [trpc] prefix.
type defaultLogger struct{}

func (defaultLogger) Info(msg string, args ...any) {
	log.Printf("[trpc] "+msg, args...)
}

func (defaultLogger) Debug(msg string, args ...any) {
	log.Printf("[trpc] "+msg, args...)
}

func (defaultLogger) Error(msg string, args ...any) {
	log.Printf("[trpc] ERROR "+msg, args...)
}

// nopLogger discards all log messages.
type nopLogger struct{}

func (nopLogger) Info(msg string, args ...any)  {}
func (nopLogger) Debug(msg string, args ...any) {}
func (nopLogger) Error(msg string, args ...any) {}

// NopLogger is a Logger that discards all messages.
// Use it with WithLogger to disable logging.
var NopLogger Logger = nopLogger{}

// LoggerFunc adapts a single printf-style function into a Logger.
// All levels (Info, Debug, Error) call the same function.
func LoggerFunc(f func(format string, args ...any)) Logger {
	return &funcLogger{f: f}
}

type funcLogger struct {
	f func(format string, args ...any)
}

func (l *funcLogger) Info(msg string, args ...any) {
	l.f("[trpc] "+msg, args...)
}

func (l *funcLogger) Debug(msg string, args ...any) {
	l.f("[trpc] "+msg, args...)
}

func (l *funcLogger) Error(msg string, args ...any) {
	l.f("[trpc] ERROR "+msg, args...)
}

// Ensure compile-time interface satisfaction.
var _ Logger = defaultLogger{}
var _ Logger = nopLogger{}
var _ Logger = (*funcLogger)(nil)
