package goai

import (
	"math"
	"testing"
)

func TestMaxTokenToInt32(t *testing.T) {
	tests := []struct {
		name     string
		maxToken int64
		want     int32
	}{
		{name: "zero", maxToken: 0, want: 0},
		{name: "typical", maxToken: 1000, want: 1000},
		{name: "negative clamps to zero", maxToken: -5, want: 0},
		{name: "max int32 unchanged", maxToken: math.MaxInt32, want: math.MaxInt32},
		{name: "overflow clamps to max int32", maxToken: math.MaxInt32 + 1, want: math.MaxInt32},
		{name: "large overflow clamps to max int32", maxToken: math.MaxInt64, want: math.MaxInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxTokenToInt32(tt.maxToken); got != tt.want {
				t.Errorf("maxTokenToInt32(%d) = %d, want %d", tt.maxToken, got, tt.want)
			}
		})
	}
}
