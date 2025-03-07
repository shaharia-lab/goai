package observability

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	// ErrorLogField is the key used for error fields in logs
	ErrorLogField string = "error"
)

// Logger interface - defines the common logging methods
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})

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

func (l *DefaultLogger) Debugf(format string, args ...interface{}) {
	l.Printf("[DEBUG] "+format, args...)
}
func (l *DefaultLogger) Infof(format string, args ...interface{}) {
	l.Printf("[INFO] "+format, args...)
}
func (l *DefaultLogger) Warnf(format string, args ...interface{}) {
	l.Printf("[WARN] "+format, args...)
}
func (l *DefaultLogger) Errorf(format string, args ...interface{}) {
	l.logWithFields("[ERROR] ", format, args...)
}
func (l *DefaultLogger) Fatalf(format string, args ...interface{}) {
	l.Fatalf("[FATAL] "+format, args...)
}
func (l *DefaultLogger) Panicf(format string, args ...interface{}) {
	l.Panicf("[PANIC] "+format, args...)
}

func (l *DefaultLogger) Debug(args ...interface{}) { l.logWithFields("[DEBUG] ", "%v", args...) }
func (l *DefaultLogger) Info(args ...interface{})  { l.logWithFields("[INFO] ", "%v", args...) }
func (l *DefaultLogger) Warn(args ...interface{})  { l.logWithFields("[WARN] ", "%v", args...) }
func (l *DefaultLogger) Error(args ...interface{}) { l.logWithFields("[ERROR] ", "%v", args...) }
func (l *DefaultLogger) Fatal(args ...interface{}) { l.logWithFields("[FATAL] ", "%v", args...) }
func (l *DefaultLogger) Panic(args ...interface{}) { l.logWithFields("[PANIC] ", "%v", args...) }

// WithFields - allows adding structured fields to the log
func (l *DefaultLogger) WithFields(fields map[string]interface{}) Logger {
	newLogger := &DefaultLogger{
		Logger: l.Logger,
		fields: make(map[string]interface{}),
		err:    l.err,
	}
	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	// Add new fields
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
		fields: l.fields, // Copy the fields
		err:    err,      // Set the error
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
	case "[FATAL] ":
		l.Logger.Fatalf(prefix+"[FATAL] "+format, args...)
	case "[PANIC] ":
		l.Logger.Panicf(prefix+"[PANIC] "+format, args...)
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

func (l *NullLogger) Debugf(format string, args ...interface{}) {}
func (l *NullLogger) Infof(format string, args ...interface{})  {}
func (l *NullLogger) Warnf(format string, args ...interface{})  {}
func (l *NullLogger) Errorf(format string, args ...interface{}) {}
func (l *NullLogger) Fatalf(format string, args ...interface{}) {}
func (l *NullLogger) Panicf(format string, args ...interface{}) {}

func (l *NullLogger) Debug(args ...interface{}) {}
func (l *NullLogger) Info(args ...interface{})  {}
func (l *NullLogger) Warn(args ...interface{})  {}
func (l *NullLogger) Error(args ...interface{}) {}
func (l *NullLogger) Fatal(args ...interface{}) {}
func (l *NullLogger) Panic(args ...interface{}) {}

func (l *NullLogger) WithFields(fields map[string]interface{}) Logger { return l }
func (l *NullLogger) WithContext(ctx context.Context) Logger          { return l }
func (l *NullLogger) WithErr(err error) Logger                        { return l }
