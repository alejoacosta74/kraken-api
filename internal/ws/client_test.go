package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/ws/test"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// internal/ws/client_test.go

func TestWebSocketClientWriterReader(t *testing.T) {
	logger.SetLevel("trace")
	// Create mock server
	mockServer := test.NewMockWebSocketServer()
	defer mockServer.Close()

	// Queue some test messages that the server will send to the client
	mockServer.QueueMessage([]byte(`{"type": "test", "data": "hello"}`))

	// Create message channel for the client
	msgChan := make(chan []byte, 10)
	errChan := make(chan error, 10)

	// Create a context with cancellation for cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create WebSocket client with mock server URL
	client := NewWebSocketClient(
		ctx,
		mockServer.Server.URL,
		msgChan,
		WithBuffers(msgChan, errChan),
		WithTradingPair("BTC/USD"),
	)

	// Run the client in a goroutine
	go func() {
		err := client.Run()
		if err != nil {
			t.Errorf("client.Run() error = %v", err)
		}
	}()

	// Wait for some time to allow connection and message exchange
	time.Sleep(100 * time.Millisecond)

	// 1. Verify subscription message was sent to server
	messages := mockServer.GetReceivedMessages()
	if len(messages) == 0 {
		t.Fatal("no subscription message received by server")
	}

	// Parse and verify subscription message
	var subMsg kraken.BookRequest
	if err := json.Unmarshal(messages[0], &subMsg); err != nil {
		t.Fatalf("failed to parse subscription message: %v", err)
	}

	// Verify subscription message contents
	if subMsg.Method != "subscribe" {
		t.Errorf("expected method 'subscribe', got %s", subMsg.Method)
	}
	if subMsg.Params.Channel != "book" {
		t.Errorf("expected channel 'book', got %s", subMsg.Params.Channel)
	}
	if len(subMsg.Params.Symbol) != 1 || subMsg.Params.Symbol[0] != "BTC/USD" {
		t.Errorf("expected symbol ['BTC/USD'], got %v", subMsg.Params.Symbol)
	}

	// 2. Verify server response was received by client
	select {
	case msg := <-msgChan:
		if string(msg) != `{"type": "test", "data": "hello"}` {
			t.Errorf("unexpected message received: %s", string(msg))
		}
	case err := <-errChan:
		t.Errorf("received error: %v", err)
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}

	// Clean up
	cancel()
}
