package mcp

import (
	"bufio"
	"encoding/json"
	"os"
)

type StdIOTransport struct {
	handler MessageHandler
	reader  *bufio.Reader
	writer  *bufio.Writer
	done    chan struct{}
}

func NewStdIOTransport() *StdIOTransport {
	return &StdIOTransport{
		reader: bufio.NewReader(os.Stdin),
		writer: bufio.NewWriter(os.Stdout),
		done:   make(chan struct{}),
	}
}

func (s *StdIOTransport) Start() error {
	// Start reading from stdin in a separate goroutine
	go s.readLoop()
	return nil
}

func (s *StdIOTransport) Stop() error {
	close(s.done)
	// Flush any remaining data
	return s.writer.Flush()
}

func (s *StdIOTransport) HandleMessage(handler MessageHandler) {
	s.handler = handler
}

func (s *StdIOTransport) readLoop() {
	for {
		select {
		case <-s.done:
			return
		default:
			// Read header line containing the content length
			line, err := s.reader.ReadString('\n')
			if err != nil {
				continue
			}

			// Parse the message
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			// Handle the message if handler is set
			if s.handler != nil {
				s.handler(msg)
			}
		}
	}
}

func (s *StdIOTransport) Send(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Write the message followed by a newline
	if _, err := s.writer.Write(append(data, '\n')); err != nil {
		return err
	}

	return s.writer.Flush()
}
