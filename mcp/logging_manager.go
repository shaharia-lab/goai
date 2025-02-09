package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

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
