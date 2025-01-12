package ai

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
				MaxToken:    1000,
				TopP:        0.9,
				Temperature: 0.7,
				TopK:        50,
			},
		},
		{
			name: "with single option",
			opts: []RequestOption{
				WithMaxToken(2000),
			},
			expected: LLMRequestConfig{
				MaxToken:    2000,
				TopP:        0.9,
				Temperature: 0.7,
				TopK:        50,
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
				MaxToken:    2000,
				TopP:        0.95,
				Temperature: 0.8,
				TopK:        100,
			},
		},
		{
			name: "with zero values - should not override defaults",
			opts: []RequestOption{
				WithMaxToken(0),
				WithTopP(0),
				WithTemperature(0),
				WithTopK(0),
			},
			expected: LLMRequestConfig{
				MaxToken:    1000,
				TopP:        0.9,
				Temperature: 0.7,
				TopK:        50,
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
				MaxToken:    1000,
				TopP:        0.9,
				Temperature: 0.7,
				TopK:        50,
			},
		},
		{
			name: "with mixed valid and invalid values",
			opts: []RequestOption{
				WithMaxToken(2000),
				WithTopP(-0.5), // invalid
				WithTemperature(0.8),
				WithTopK(0), // invalid
			},
			expected: LLMRequestConfig{
				MaxToken:    2000,
				TopP:        0.9, // keeps default
				Temperature: 0.8,
				TopK:        50, // keeps default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewRequestConfig(tt.opts...)

			if result.MaxToken != tt.expected.MaxToken {
				t.Errorf("MaxToken: expected %d, got %d", tt.expected.MaxToken, result.MaxToken)
			}
			if result.TopP != tt.expected.TopP {
				t.Errorf("TopP: expected %f, got %f", tt.expected.TopP, result.TopP)
			}
			if result.Temperature != tt.expected.Temperature {
				t.Errorf("Temperature: expected %f, got %f", tt.expected.Temperature, result.Temperature)
			}
			if result.TopK != tt.expected.TopK {
				t.Errorf("TopK: expected %d, got %d", tt.expected.TopK, result.TopK)
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
		{"zero value", 0, false},
		{"negative value", -100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithMaxToken(tt.input)(&config)

			if tt.shouldApply {
				if config.MaxToken != tt.input {
					t.Errorf("expected MaxToken to be %d, got %d", tt.input, config.MaxToken)
				}
			} else {
				if config.MaxToken != DefaultConfig.MaxToken {
					t.Errorf("expected MaxToken to remain %d, got %d", DefaultConfig.MaxToken, config.MaxToken)
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
		{"zero value", 0.0, false},
		{"negative value", -0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTopP(tt.input)(&config)

			if tt.shouldApply {
				if config.TopP != tt.input {
					t.Errorf("expected TopP to be %f, got %f", tt.input, config.TopP)
				}
			} else {
				if config.TopP != DefaultConfig.TopP {
					t.Errorf("expected TopP to remain %f, got %f", DefaultConfig.TopP, config.TopP)
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
		{"zero value", 0.0, false},
		{"negative value", -0.3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTemperature(tt.input)(&config)

			if tt.shouldApply {
				if config.Temperature != tt.input {
					t.Errorf("expected Temperature to be %f, got %f", tt.input, config.Temperature)
				}
			} else {
				if config.Temperature != DefaultConfig.Temperature {
					t.Errorf("expected Temperature to remain %f, got %f", DefaultConfig.Temperature, config.Temperature)
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
		{"zero value", 0, false},
		{"negative value", -10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig
			WithTopK(tt.input)(&config)

			if tt.shouldApply {
				if config.TopK != tt.input {
					t.Errorf("expected TopK to be %d, got %d", tt.input, config.TopK)
				}
			} else {
				if config.TopK != DefaultConfig.TopK {
					t.Errorf("expected TopK to remain %d, got %d", DefaultConfig.TopK, config.TopK)
				}
			}
		})
	}
}
