// utils.go
package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/google/uuid"
	"time"
)

// generateResourceURI generates a unique URI for a resource
func generateResourceURI() string {
	// Generate 16 random bytes
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based URI if random generation fails
		return "resource-" + time.Now().Format("20060102-150405.000")
	}

	return "resource-" + hex.EncodeToString(b)
}

// getCurrentTimestamp returns the current Unix timestamp in milliseconds
func getCurrentTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// generateClientID generates a unique client identifier
func generateClientID() string {
	return uuid.New().String()
}

func generateRequestID() string {
	return uuid.New().String()
}
