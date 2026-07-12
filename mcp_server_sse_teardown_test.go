package goai

import (
	"sync"
	"testing"
)

// TestCloseClientLockedIdempotent verifies that a client's channel is closed
// exactly once even when the two teardown paths race. It reproduces the #77
// scenario: many goroutines each call closeClientLocked (under the mutex) for
// the same client. Before the fix this double-closed the channel and paniced
// with "close of closed channel"; now the second and later calls are no-ops.
func TestCloseClientLockedIdempotent(t *testing.T) {
	s := &SSEServer{clients: make(map[string]chan []byte)}
	clientID := "c1"
	s.clients[clientID] = make(chan []byte, 1)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			// Mirror the real teardown paths, which hold clientsMutex.
			s.clientsMutex.Lock()
			s.closeClientLocked(clientID)
			s.clientsMutex.Unlock()
		}()
	}
	wg.Wait() // a double-close would have paniced here

	s.clientsMutex.RLock()
	_, exists := s.clients[clientID]
	s.clientsMutex.RUnlock()
	if exists {
		t.Fatal("client should have been removed from the registry after close")
	}
}

// TestRemoveClientIdempotent ensures removeClient can be called repeatedly for
// the same client (as the send-timeout and receive-loop paths may) without
// panicking.
func TestRemoveClientIdempotent(t *testing.T) {
	s := &SSEServer{clients: make(map[string]chan []byte)}
	s.clients["c1"] = make(chan []byte, 1)

	s.removeClient("c1")
	s.removeClient("c1") // second call must be a safe no-op
	s.removeClient("never-registered")
}
