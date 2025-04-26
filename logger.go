package goai

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// ErrorLogField is the key used for error fields in logs
	ErrorLogField string = "error"
)

// Logger interface - defines the common logging methods
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})

	WithFields(fields map[string]interface{}) Logger
	WithContext(ctx context.Context) Logger
	WithErr(err error) Logger
}

// DefaultLogger - a basic implementation using Go's standard log package
type DefaultLogger struct {
	*log.Logger
	fields map[string]interface{}
	err    error
}

// NewDefaultLogger creates a new DefaultLogger that logs to standard output
func NewDefaultLogger() Logger {
	return &DefaultLogger{
		Logger: log.New(os.Stdout, "", log.LstdFlags),
		fields: make(map[string]interface{}),
	}
}

func (l *DefaultLogger) Debug(args ...interface{}) { l.logWithFields("[DEBUG] ", "%v", args...) }
func (l *DefaultLogger) Info(args ...interface{})  { l.logWithFields("[INFO] ", "%v", args...) }
func (l *DefaultLogger) Warn(args ...interface{})  { l.logWithFields("[WARN] ", "%v", args...) }
func (l *DefaultLogger) Error(args ...interface{}) { l.logWithFields("[ERROR] ", "%v", args...) }

// WithFields - allows adding structured fields to the log
func (l *DefaultLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &DefaultLogger{
		Logger: l.Logger,
		fields: make(map[string]interface{}),
		err:    l.err,
	}

	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

// WithContext - No-op for DefaultLogger. Returns itself.
func (l *DefaultLogger) WithContext(ctx context.Context) Logger {
	// In a real-world scenario, you might extract values from the context
	// and add them to the fields. For this example, we'll just return
	// the logger as is.
	return l
}

// WithErr - allows adding an error to the log
func (l *DefaultLogger) WithErr(err error) Logger {
	return &DefaultLogger{
		Logger: l.Logger,
		fields: l.fields,
		err:    err,
	}
}

// Helper function to log with fields and error
func (l *DefaultLogger) logWithFields(level string, format string, args ...interface{}) {
	var parts []string
	for k, v := range l.fields {
		parts = append(parts, fmt.Sprintf("%v=%v", k, v))
	}
	if l.err != nil {
		parts = append(parts, fmt.Sprintf("error=%v", l.err))
		args = append(args, l.err)
	}
	prefix := ""
	if len(parts) > 0 {
		prefix = fmt.Sprintf("[%s] ", strings.Join(parts, " "))
	}

	switch level {
	case "[DEBUG] ":
		l.Logger.Printf(prefix+"[DEBUG] "+format, args...)
	case "[INFO] ":
		l.Logger.Printf(prefix+"[INFO] "+format, args...)
	case "[WARN] ":
		l.Logger.Printf(prefix+"[WARN] "+format, args...)
	case "[ERROR] ":
		l.Logger.Printf(prefix+"[ERROR] "+format, args...)
	default:
		l.Logger.Printf(prefix+level+format, args...)
	}
}

// NullLogger - a logger that does nothing
type NullLogger struct{}

// NewNullLogger creates a new NullLogger
func NewNullLogger() Logger {
	return &NullLogger{}
}

// Debug is a no-op for NullLogger
func (l *NullLogger) Debug(args ...interface{}) {}

// Info is a no-op for NullLogger
func (l *NullLogger) Info(args ...interface{}) {}

// Warn is a no-op for NullLogger
func (l *NullLogger) Warn(args ...interface{}) {}

// Error is a no-op for NullLogger
func (l *NullLogger) Error(args ...interface{}) {}

// WithFields is a no-op for NullLogger
func (l *NullLogger) WithFields(fields map[string]interface{}) Logger { return l }

// WithContext is a no-op for NullLogger
func (l *NullLogger) WithContext(ctx context.Context) Logger { return l }

// WithErr is a no-op for NullLogger
func (l *NullLogger) WithErr(err error) Logger { return l }

// SlogLogger implements the Logger interface using the standard library's slog package
type SlogLogger struct {
	logger *slog.Logger
	attrs  []any
}

// NewSlogLogger creates a new SlogLogger with the provided slog.Logger
func NewSlogLogger(logger *slog.Logger) Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &SlogLogger{
		logger: logger,
		attrs:  []any{},
	}
}

// Debug log for SlogLogger
func (l *SlogLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Info log for SlogLogger
func (l *SlogLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Warn log for SlogLogger
func (l *SlogLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Error log for SlogLogger
func (l *SlogLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// WithFields adds fields to the logger and returns a new SlogLogger
func (l *SlogLogger) WithFields(fields map[string]interface{}) Logger {
	// Create a copy of the existing attributes
	attrs := make([]any, len(l.attrs))
	copy(attrs, l.attrs)

	// Add the new fields
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	return &SlogLogger{
		logger: l.logger.With(attrs...),
		attrs:  attrs,
	}
}

// WithContext adds context to the logger and returns a new SlogLogger
func (l *SlogLogger) WithContext(ctx context.Context) Logger {
	return &SlogLogger{
		logger: l.logger,
		attrs:  l.attrs,
	}
}

// WithErr adds an error to the logger and returns a new SlogLogger
func (l *SlogLogger) WithErr(err error) Logger {
	return &SlogLogger{
		logger: l.logger.With(slog.Any(ErrorLogField, err)),
		attrs:  append(append([]any{}, l.attrs...), slog.Any(ErrorLogField, err)),
	}
}

// LogrusLogger implements the Logger interface using logrus
type LogrusLogger struct {
	entry *logrus.Entry
}

// NewLogrusLogger creates a new LogrusLogger with the provided logrus.Logger
func NewLogrusLogger(logger *logrus.Logger) Logger {
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &LogrusLogger{
		entry: logrus.NewEntry(logger),
	}
}

// Debug log for LogrusLogger
func (l *LogrusLogger) Debug(args ...interface{}) {
	l.entry.Debug(args...)
}

// Info log for LogrusLogger
func (l *LogrusLogger) Info(args ...interface{}) {
	l.entry.Info(args...)
}

// Warn log for LogrusLogger
func (l *LogrusLogger) Warn(args ...interface{}) {
	l.entry.Warn(args...)
}

// Error log for LogrusLogger
func (l *LogrusLogger) Error(args ...interface{}) {
	l.entry.Error(args...)
}

// WithFields adds fields to the logger and returns a new LogrusLogger
func (l *LogrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &LogrusLogger{
		entry: l.entry.WithFields(logrus.Fields(fields)),
	}
}

// WithContext adds context to the logger and returns a new LogrusLogger
func (l *LogrusLogger) WithContext(ctx context.Context) Logger {
	return &LogrusLogger{
		entry: l.entry.WithContext(ctx),
	}
}

// WithErr adds an error to the logger and returns a new LogrusLogger
func (l *LogrusLogger) WithErr(err error) Logger {
	return &LogrusLogger{
		entry: l.entry.WithError(err),
	}
}

// ZapLogger implements the Logger interface using uber-go/zap
type ZapLogger struct {
	logger *zap.Logger
	sugar  *zap.SugaredLogger
	fields []zapcore.Field
}

// NewZapLogger creates a new ZapLogger with the provided zap.Logger
func NewZapLogger(logger *zap.Logger) Logger {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &ZapLogger{
		logger: logger,
		sugar:  logger.Sugar(),
		fields: []zapcore.Field{},
	}
}

// Debug log for ZapLogger
func (l *ZapLogger) Debug(args ...interface{}) {
	l.sugar.Debug(args...)
}

// Info log for ZapLogger
func (l *ZapLogger) Info(args ...interface{}) {
	l.sugar.Info(args...)
}

// Warn log for ZapLogger
func (l *ZapLogger) Warn(args ...interface{}) {
	l.sugar.Warn(args...)
}

// Error log for ZapLogger
func (l *ZapLogger) Error(args ...interface{}) {
	l.sugar.Error(args...)
}

// WithFields adds fields to the logger and returns a new ZapLogger
func (l *ZapLogger) WithFields(fields map[string]interface{}) Logger {
	zapFields := make([]zapcore.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return &ZapLogger{
		logger: l.logger.With(zapFields...),
		sugar:  l.logger.With(zapFields...).Sugar(),
		fields: append(l.fields, zapFields...),
	}
}

// WithContext adds context to the logger and returns a new ZapLogger
func (l *ZapLogger) WithContext(ctx context.Context) Logger {
	return l
}

// WithErr adds an error to the logger and returns a new ZapLogger
func (l *ZapLogger) WithErr(err error) Logger {
	return &ZapLogger{
		logger: l.logger.With(zap.Error(err)),
		sugar:  l.logger.With(zap.Error(err)).Sugar(),
		fields: append(l.fields, zap.Error(err)),
	}
}
