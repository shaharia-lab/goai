package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

func (s *StdIOTransport) readLoop() {
	for {
		select {
		case <-s.done:
			return
		default:
			// Read the Content-Length header
			header, err := s.reader.ReadString('\n')
			if err != nil {
				continue
			}
			header = strings.TrimSpace(header)
			if !strings.HasPrefix(header, "Content-Length: ") {
				continue
			}

			// Parse content length
			var contentLength int
			_, err = fmt.Sscanf(header, "Content-Length: %d", &contentLength)
			if err != nil {
				continue
			}

			// Read the empty line
			_, err = s.reader.ReadString('\n')
			if err != nil {
				continue
			}

			// Read the message content
			content := make([]byte, contentLength)
			_, err = s.reader.Read(content)
			if err != nil {
				continue
			}

			// Parse the message
			var msg Message
			if err := json.Unmarshal(content, &msg); err != nil {
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

	// Write the headers
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := s.writer.WriteString(header); err != nil {
		return err
	}

	// Write the message
	if _, err := s.writer.Write(data); err != nil {
		return err
	}

	return s.writer.Flush()
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

func (t *StdIOTransport) SendMessage(msg Message) error {
	return json.NewEncoder(os.Stdout).Encode(msg)
}
