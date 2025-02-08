// logging.go

package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// LogLevel represents the severity level of a log message.
// The levels follow standard syslog severity levels.
type LogLevel string

const (
	// LogLevelDebug represents debug-level messages (detailed debug information)
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo represents informational messages
	LogLevelInfo LogLevel = "info"
	// LogLevelNotice represents normal but significant conditions
	LogLevelNotice LogLevel = "notice"
	// LogLevelWarning represents warning conditions
	LogLevelWarning LogLevel = "warning"
	// LogLevelError represents error conditions
	LogLevelError LogLevel = "error"
	// LogLevelCritical represents critical conditions
	LogLevelCritical LogLevel = "critical"
	// LogLevelAlert represents conditions that should be corrected immediately
	LogLevelAlert LogLevel = "alert"
	// LogLevelEmergency represents system is unusable
	LogLevelEmergency LogLevel = "emergency"
)

// logLevelSeverity maps LogLevel to their numeric severity values.
// Lower numbers indicate higher severity (0 is most severe).
var logLevelSeverity = map[LogLevel]int{
	LogLevelDebug:     7,
	LogLevelInfo:      6,
	LogLevelNotice:    5,
	LogLevelWarning:   4,
	LogLevelError:     3,
	LogLevelCritical:  2,
	LogLevelAlert:     1,
	LogLevelEmergency: 0,
}

// SetLogLevelParams represents the parameters for setting the log level.
type SetLogLevelParams struct {
	Level LogLevel `json:"level"`
}

// LogMessageParams represents the parameters for logging a message.
type LogMessageParams struct {
	Level  LogLevel    `json:"level"`
	Logger string      `json:"logger,omitempty"`
	Data   interface{} `json:"data"`
}

// LogManager handles logging operations and level management.
type LogManager struct {
	currentLevel LogLevel
	output       io.Writer
}

// NewLogManager creates a new LogManager with the specified output writer.
// If output is nil, os.Stderr will be used.
func NewLogManager(output io.Writer) *LogManager {
	if output == nil {
		output = os.Stderr
	}
	return &LogManager{
		currentLevel: LogLevelInfo, // Default level
		output:       output,
	}
}

// SetLevel sets the current logging level.
func (lm *LogManager) SetLevel(level LogLevel) error {
	if _, ok := logLevelSeverity[level]; !ok {
		return fmt.Errorf("invalid log level: %s", level)
	}
	lm.currentLevel = level
	return nil
}

// IsLevelEnabled checks if a given log level is enabled based on the current level.
func (lm *LogManager) IsLevelEnabled(level LogLevel) bool {
	currentSeverity := logLevelSeverity[lm.currentLevel]
	msgSeverity := logLevelSeverity[level]
	return msgSeverity <= currentSeverity
}

// Log logs a message if its level is enabled.
func (lm *LogManager) Log(params LogMessageParams) error {
	if !lm.IsLevelEnabled(params.Level) {
		return nil
	}

	msg := struct {
		Timestamp string      `json:"timestamp"`
		Level     LogLevel    `json:"level"`
		Logger    string      `json:"logger,omitempty"`
		Data      interface{} `json:"data"`
	}{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     params.Level,
		Logger:    params.Logger,
		Data:      params.Data,
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal log message: %w", err)
	}

	_, err = fmt.Fprintln(lm.output, string(encoded))
	return err
}
