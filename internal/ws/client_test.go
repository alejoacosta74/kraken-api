package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alejoacosta74/kraken-api/internal/ws/mocks"
	"github.com/alejoacosta74/kraken-api/pkg/kraken"
)

// internal/ws/client_test.go

func TestWebSocketClientWriterReader(t *testing.T) {

	testCases := []struct {
		name           string
		tradingPair    string
		messagesToSend []byte
		setupHandler   func(mockServer *mocks.MockWebSocketServer)
		wantErr        bool
		expectedErr    string
	}{
		{
			name:           "BTC/USD succesfull subscription",
			tradingPair:    "BTC/USD",
			messagesToSend: nil,
			setupHandler: func(mockServer *mocks.MockWebSocketServer) {
				mockServer.RegisterHandler("subscribe", func(msg []byte) interface{} {
					return kraken.BookResponse{
						Method:  "subscribe",
						Success: true,
						Result: kraken.BookResult{
							Channel:  "book",
							Symbol:   "BTC/USD",
							Depth:    10,
							Snapshot: true,
						},
					}
				})
			},
			wantErr:     false,
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock server
			mockServer := mocks.NewMockWebSocketServer()
			defer mockServer.Close()

			if tc.setupHandler != nil {
				tc.setupHandler(mockServer)
			}

			if tc.messagesToSend != nil {
				mockServer.QueueMessage(tc.messagesToSend)
			}

			// Create a context with cancellation for cleanup
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Create config for the client
			cfg := ClientConfig{
				Ctx:      ctx,
				Url:      mockServer.Server.URL,
				MsgChan:  make(chan []byte, 10),
				ErrChan:  make(chan error, 10),
				DoneChan: make(chan struct{}),
				Opts:     []Option{WithTradingPair("BTC/USD")},
			}

			// Create WebSocket client with mock server URL
			client := NewWebSocketClient(cfg)

			// Run the client in a goroutine
			go func() {
				err := client.Run()
				if err != nil {
					t.Logf("client.Run() error = %v", err)
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
			if len(subMsg.Params.Symbol) != 1 || subMsg.Params.Symbol[0] != tc.tradingPair {
				t.Errorf("expected symbol ['BTC/USD'], got %v", subMsg.Params.Symbol)
			}
			// Clean up
			cancel()
		})
	}
}

func TestWebSocketClientGracefulShutdown(t *testing.T) {
	// Create mock server
	mockServer := mocks.NewMockWebSocketServer()
	defer mockServer.Close()

	// Setup success handler for subscription
	mockServer.RegisterHandler("subscribe", func(msg []byte) interface{} {
		return kraken.BookResponse{
			Method:  "subscribe",
			Success: true,
			Result: kraken.BookResult{
				Channel:  "book",
				Symbol:   "BTC/USD",
				Depth:    10,
				Snapshot: true,
			},
		}
	})

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	msgChan := make(chan []byte, 10)
	errChan := make(chan error, 10)
	doneChan := make(chan struct{})
	// create config for the client
	cfg := ClientConfig{
		Ctx:      ctx,
		Url:      mockServer.Server.URL,
		MsgChan:  msgChan,
		ErrChan:  errChan,
		DoneChan: doneChan,
		Opts:     []Option{WithTradingPair("BTC/USD")},
	}

	// Create client
	client := NewWebSocketClient(cfg)

	go func() {
		if err := client.Run(); err != nil {
			t.Logf("client.Run() error = %v", err)
		}
	}()

	// Wait for client to establish connection and subscribe
	time.Sleep(100 * time.Millisecond)

	// Initiate graceful shutdown
	cancel()

	// Wait for shutdown with timeout
	select {
	case <-doneChan:
		t.Log("client shutdown")
	case <-time.After(6 * time.Second): // Slightly longer than internal timeout
		t.Fatal("shutdown timeout: client failed to shut down gracefully")
	}

	// Verify channels are drained
	select {
	case _, ok := <-msgChan:
		if ok {
			t.Error("message channel should be empty")
		}
	default:
	}

	select {
	case _, ok := <-errChan:
		if ok {
			t.Error("error channel should be empty")
		}
	default:
	}
}
