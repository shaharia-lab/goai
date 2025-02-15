package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type StdIOTransport struct {
	config          StdIOConfig
	logger          *log.Logger
	stopChan        chan struct{}
	mu              sync.RWMutex
	reader          *bufio.Reader
	writer          io.Writer
	receiveCallback func(message []byte)
	state           ConnectionState
}

func NewStdIOTransport() *StdIOTransport {
	return &StdIOTransport{
		stopChan: make(chan struct{}),
		logger:   log.Default(),
		state:    Disconnected,
	}
}

func (t *StdIOTransport) SetReceiveMessageCallback(callback func(message []byte)) {
	t.receiveCallback = callback
}

func (t *StdIOTransport) Connect(config ClientConfig) error {
	t.config = config.StdIO
	t.logger = config.Logger
	t.state = Connecting

	if t.config.Reader == nil {
		t.config.Reader = os.Stdin
	}
	if t.config.Writer == nil {
		t.config.Writer = os.Stdout
	}

	t.reader = bufio.NewReader(t.config.Reader)
	t.writer = t.config.Writer
	t.state = Connected

	go t.processIncomingMessages()
	return nil
}

func (t *StdIOTransport) processIncomingMessages() {
	scanner := bufio.NewScanner(t.reader)
	for scanner.Scan() {
		select {
		case <-t.stopChan:
			return
		default:
			line := scanner.Text()
			t.logger.Printf("Received raw input: %s", line)
			t.receiveCallback([]byte(line))
		}
	}

	if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "file already closed") {
		t.logger.Printf("Scanner error: %v", err)
	}
}

func (t *StdIOTransport) SendMessage(message interface{}) error {
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	jsonData = append(jsonData, '\n')
	t.logger.Printf("Sending message: %s", string(jsonData))

	_, err = t.writer.Write(jsonData)
	if err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

func (t *StdIOTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Disconnected {
		return nil
	}

	t.state = Disconnected
	t.logger.Println("Shutting down StdIO transport...")
	close(t.stopChan)
	t.logger.Println("StdIO transport shutdown complete")
	return nil
}
