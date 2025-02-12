package mcp

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewLogManager(t *testing.T) {
	tests := []struct {
		name   string
		output io.Writer
	}{
		{
			name:   "with nil output",
			output: nil,
		},
		{
			name:   "with custom output",
			output: &bytes.Buffer{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lm := NewLogManager(tt.output)
			if lm == nil {
				t.Error("NewLogManager returned nil")
			}
			if tt.output == nil && lm.output != os.Stderr {
				t.Error("Expected os.Stderr as default output")
			}
			if tt.output != nil && lm.output != tt.output {
				t.Error("Output not set correctly")
			}
		})
	}
}

func TestLogManager_SetLevel(t *testing.T) {
	tests := []struct {
		name          string
		level         LogLevel
		expectedError bool
	}{
		{
			name:          "valid level - debug",
			level:         LogLevelDebug,
			expectedError: false,
		},
		{
			name:          "valid level - info",
			level:         LogLevelInfo,
			expectedError: false,
		},
		{
			name:          "invalid level",
			level:         LogLevel("INVALID"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lm := NewLogManager(nil)
			err := lm.SetLevel(tt.level)

			if tt.expectedError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectedError {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if lm.currentLevel != tt.level {
					t.Errorf("Level not set correctly, got %v, want %v", lm.currentLevel, tt.level)
				}
			}
		})
	}
}

func TestLogManager_IsLevelEnabled(t *testing.T) {
	tests := []struct {
		name          string
		currentLevel  LogLevel
		checkLevel    LogLevel
		expectEnabled bool
	}{
		{
			name:          "debug enables debug",
			currentLevel:  LogLevelDebug,
			checkLevel:    LogLevelDebug,
			expectEnabled: true,
		},
		{
			name:          "info disables debug",
			currentLevel:  LogLevelInfo,
			checkLevel:    LogLevelDebug,
			expectEnabled: false,
		},
		{
			name:          "error enables error",
			currentLevel:  LogLevelError,
			checkLevel:    LogLevelError,
			expectEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lm := NewLogManager(nil)
			err := lm.SetLevel(tt.currentLevel)
			assert.NoError(t, err)

			if enabled := lm.IsLevelEnabled(tt.checkLevel); enabled != tt.expectEnabled {
				t.Errorf("IsLevelEnabled() = %v, want %v", enabled, tt.expectEnabled)
			}
		})
	}
}

func TestLogManager_Log(t *testing.T) {
	buf := &bytes.Buffer{}
	lm := NewLogManager(buf)

	testData := "test message"
	testLogger := "test_logger"

	err := lm.Log(LogMessageParams{
		Level:  LogLevelInfo,
		Logger: testLogger,
		Data:   testData,
	})

	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	var loggedMsg struct {
		Timestamp string      `json:"timestamp"`
		Level     LogLevel    `json:"level"`
		Logger    string      `json:"logger"`
		Data      interface{} `json:"data"`
	}

	output := strings.TrimSpace(buf.String())
	if err := json.Unmarshal([]byte(output), &loggedMsg); err != nil {
		t.Errorf("Failed to parse JSON output: %v", err)
	}

	// Verify fields
	if loggedMsg.Level != LogLevelInfo {
		t.Errorf("Wrong log level in output: got %v, want %v", loggedMsg.Level, LogLevelInfo)
	}
	if loggedMsg.Logger != testLogger {
		t.Errorf("Wrong logger in output: got %v, want %v", loggedMsg.Logger, testLogger)
	}
	if loggedMsg.Data != testData {
		t.Errorf("Wrong data in output: got %v, want %v", loggedMsg.Data, testData)
	}

	_, err = time.Parse(time.RFC3339, loggedMsg.Timestamp)
	if err != nil {
		t.Errorf("Invalid timestamp format: %v", err)
	}
}

func TestLogManager_LogLevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	lm := NewLogManager(buf)
	err := lm.SetLevel(LogLevelError)
	assert.NoError(t, err)

	err = lm.Log(LogMessageParams{
		Level: LogLevelInfo,
		Data:  "should not be logged",
	})

	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	if buf.Len() > 0 {
		t.Error("Expected no output for filtered log level")
	}

	err = lm.Log(LogMessageParams{
		Level: LogLevelError,
		Data:  "should be logged",
	})

	if err != nil {
		t.Errorf("Log() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Expected output for matching log level")
	}
}
