package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/shaharia-lab/goai/observability"
	"go.opentelemetry.io/otel/codes"
)

type StdIOTransport struct {
	config          StdIOConfig
	logger          observability.Logger
	stopChan        chan struct{}
	mu              sync.RWMutex
	reader          *bufio.Reader
	writer          io.Writer
	receiveCallback func(message []byte)
	state           ConnectionState
}

func NewStdIOTransport(logger observability.Logger) *StdIOTransport {
	return &StdIOTransport{
		stopChan: make(chan struct{}),
		logger:   logger,
		state:    Disconnected,
	}
}

func (t *StdIOTransport) SetReceiveMessageCallback(callback func(message []byte)) {
	t.receiveCallback = callback
}

func (t *StdIOTransport) Connect(ctx context.Context, config ClientConfig) error {
	ctx, span := observability.StartSpan(ctx, "StdIOTransport.Connect")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

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

	go t.processIncomingMessages(ctx)
	return nil
}

func (t *StdIOTransport) processIncomingMessages(ctx context.Context) {
	scanner := bufio.NewScanner(t.reader)
	for scanner.Scan() {
		select {
		case <-t.stopChan:
			return
		default:
			line := scanner.Text()
			t.logger.Debugf("Received raw input")
			t.receiveCallback([]byte(line))
		}
	}

	if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "file already closed") {
		t.logger.WithErr(err).Errorf("StdIOTransport Scanner error")
	}
}

func (t *StdIOTransport) SendMessage(ctx context.Context, message interface{}) error {
	ctx, span := observability.StartSpan(ctx, "StdIOTransport.SendMessage")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	jsonData = append(jsonData, '\n')
	t.logger.Debug("StdIOTransport: Sending message")

	_, err = t.writer.Write(jsonData)
	if err != nil {
		t.logger.WithErr(err).Error("Failed to write message")
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

func (t *StdIOTransport) Close(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx, "StdIOTransport.Close")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Disconnected {
		return nil
	}

	t.state = Disconnected
	t.logger.Debug("Shutting down StdIO transport...")
	close(t.stopChan)
	t.logger.Debug("StdIO transport shutdown complete")
	return nil
}
