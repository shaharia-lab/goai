package goai

import (
	"testing"
)

func TestNewRequestConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     []RequestOption
		expected LLMRequestConfig
	}{
		{
			name: "no options - should use defaults",
			expected: LLMRequestConfig{
				maxToken:      1000,
				topP:          0.5,
				temperature:   0.5,
				topK:          40,
				enableTracing: false,
				toolsProvider: NewToolsProvider(),
			},
		},
		{
			name: "with single option",
			opts: []RequestOption{
				WithMaxToken(2000),
			},
			expected: LLMRequestConfig{
				maxToken:    2000,
				topP:        0.5,
				temperature: 0.5,
				topK:        40,
			},
		},
		{
			name: "with multiple options",
			opts: []RequestOption{
				WithMaxToken(2000),
				WithTopP(0.95),
				WithTemperature(0.8),
				WithTopK(100),
			},
			expected: LLMRequestConfig{
				maxToken:    2000,
				topP:        0.95,
				temperature: 0.8,
				topK:        100,
			},
		},
		{
			name: "with zero values - should override defaults",
			opts: []RequestOption{
				WithMaxToken(0),
				WithTopP(0),
				WithTemperature(0),
				WithTopK(0),
			},
			expected: LLMRequestConfig{
				maxToken:    0,
				topP:        0,
				temperature: 0,
				topK:        0,
			},
		},
		{
			name: "with negative values - should not override defaults",
			opts: []RequestOption{
				WithMaxToken(-100),
				WithTopP(-0.5),
				WithTemperature(-0.3),
				WithTopK(-10),
			},
			expected: LLMRequestConfig{
				maxToken:    -100,
				topP:        -0.5,
				temperature: -0.3,
				topK:        -10,
			},
		},
		{
			name: "with mixed valid and invalid values",
			opts: []RequestOption{
				WithMaxToken(2000),
				WithTopP(-0.5),
				WithTemperature(0.8),
				WithTopK(0),
			},
			expected: LLMRequestConfig{
				maxToken:    2000,
				topP:        -0.5,
				temperature: 0.8,
				topK:        0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewRequestConfig(tt.opts...)

			if result.maxToken != tt.expected.maxToken {
				t.Errorf("maxToken: expected %d, got %d", tt.expected.maxToken, result.maxToken)
			}
			if result.topP != tt.expected.topP {
				t.Errorf("topP: expected %f, got %f", tt.expected.topP, result.topP)
			}
			if result.temperature != tt.expected.temperature {
				t.Errorf("temperature: expected %f, got %f", tt.expected.temperature, result.temperature)
			}
			if result.topK != tt.expected.topK {
				t.Errorf("topK: expected %d, got %d", tt.expected.topK, result.topK)
			}
		})
	}
}

// Individual option tests
func TestWithMaxToken(t *testing.T) {
	tests := []struct {
		name        string
		input       int64
		shouldApply bool
	}{
		{"positive value", 2000, true},
		{"zero value", 0, true},
		{"negative value", -100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithMaxToken(tt.input)(&config)

			if tt.shouldApply {
				if config.maxToken != tt.input {
					t.Errorf("expected maxToken to be %d, got %d", tt.input, config.maxToken)
				}
			} else {
				if config.maxToken != DefaultConfig.maxToken {
					t.Errorf("expected maxToken to remain %d, got %d", DefaultConfig.maxToken, config.maxToken)
				}
			}
		})
	}
}

func TestWithTopP(t *testing.T) {
	tests := []struct {
		name        string
		input       float64
		shouldApply bool
	}{
		{"valid value", 0.95, true},
		{"zero value", 0.0, true},
		{"negative value", -0.5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTopP(tt.input)(&config)

			if tt.shouldApply {
				if config.topP != tt.input {
					t.Errorf("expected topP to be %f, got %f", tt.input, config.topP)
				}
			} else {
				if config.topP != DefaultConfig.topP {
					t.Errorf("expected topP to remain %f, got %f", DefaultConfig.topP, config.topP)
				}
			}
		})
	}
}

func TestWithTemperature(t *testing.T) {
	tests := []struct {
		name        string
		input       float64
		shouldApply bool
	}{
		{"valid value", 0.8, true},
		{"zero value", 0.0, true},
		{"negative value", -0.3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTemperature(tt.input)(&config)

			if tt.shouldApply {
				if config.temperature != tt.input {
					t.Errorf("expected temperature to be %f, got %f", tt.input, config.temperature)
				}
			} else {
				if config.temperature != DefaultConfig.temperature {
					t.Errorf("expected temperature to remain %f, got %f", DefaultConfig.temperature, config.temperature)
				}
			}
		})
	}
}

func TestWithTopK(t *testing.T) {
	tests := []struct {
		name        string
		input       int64
		shouldApply bool
	}{
		{"valid value", 100, true},
		{"zero value", 0, true},
		{"negative value", -10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTopK(tt.input)(&config)

			if tt.shouldApply {
				if config.topK != tt.input {
					t.Errorf("expected topK to be %d, got %d", tt.input, config.topK)
				}
			} else {
				if config.topK != DefaultConfig.topK {
					t.Errorf("expected topK to remain %d, got %d", DefaultConfig.topK, config.topK)
				}
			}
		})
	}
}
