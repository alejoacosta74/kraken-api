package dispatcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/common"
	"github.com/alejoacosta74/kraken-api/internal/dispatcher/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDispatcher_Run(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockProducerPool := mocks.NewMockProducerPool(ctrl)
	mockEventBus := mocks.NewMockBus(ctrl)
	mockHandler := mocks.NewMockMessageHandler(ctrl)

	// Test cases
	tests := []struct {
		name           string
		message        []byte
		setupMockStubs func()
		expectedError  bool
	}{
		{
			name:    "successful message processing",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil).Times(1)
				mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedError: false,
		},
		{
			name:    "handler error",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
				mockHandler.EXPECT().Handle(gomock.Any()).Return(assert.AnError).Times(1)
			},
			expectedError: true,
		},
		{
			name:    "timeout waiting for producer pool to stop",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				// Simulate a long running producer pool stop
				mockProducerPool.EXPECT().Stop().Times(1).DoAndReturn(func() {
					time.Sleep(6 * time.Second) // Sleep longer than the dispatcher's timeout
				})
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil).Times(1)
				mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// comment / uncomment to see logs
			logger.SetLevel("trace")

			// Create channels
			msgChan := make(chan []byte, 1) // channel to receive messages
			errChan := make(chan error, 1)  // channel to receive errors
			doneChan := make(chan struct{}) // channel to wait for dispatcher to finish

			// Create context
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Create dispatcher config
			cfg := DispatcherConfig{
				MsgChan:      msgChan,
				ErrChan:      errChan,
				DoneChan:     doneChan,
				EventBus:     mockEventBus,
				ProducerPool: mockProducerPool,
				Ctx:          ctx,
			}

			// Create dispatcher and register handler
			dispatcher := NewDispatcher(cfg)
			dispatcher.RegisterHandler(common.TypeBookSnapshot, mockHandler)

			// Setup mocks
			tt.setupMockStubs()

			// Start dispatcher in goroutine
			go dispatcher.Run()

			// Send test message
			msgChan <- tt.message

			// Wait for processing
			time.Sleep(1 * time.Second)

			// Cancel context
			cancel()

			// Check for errors
			if tt.expectedError {
				select {
				case err := <-errChan:
					assert.Error(t, err)
				case <-time.After(6 * time.Second):
					t.Error("timeout waiting for error")
				}
			}

			// Wait for dispatcher to finish
			select {
			case <-doneChan:
				// Success
			case <-time.After(6 * time.Second):
				t.Error("timeout waiting for dispatcher to finish")
			}
		})
	}
}

func TestDispatcher_dispatch(t *testing.T) {
	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockProducerPool := mocks.NewMockProducerPool(ctrl)
	mockEventBus := mocks.NewMockBus(ctrl)
	mockHandler := mocks.NewMockMessageHandler(ctrl)

	// Create test dispatcher
	cfg := DispatcherConfig{
		MsgChan:      make(chan []byte),
		ErrChan:      make(chan error),
		DoneChan:     make(chan struct{}),
		EventBus:     mockEventBus,
		ProducerPool: mockProducerPool,
	}
	d := NewDispatcher(cfg)

	tests := []struct {
		name           string
		message        []byte
		setupMockStubs func()
		registerTypes  map[common.MessageType]MessageHandler
		expectedError  string
	}{
		{
			name:    "successful book snapshot dispatch",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {"symbol": "XBT/USD"}}`),
			setupMockStubs: func() {
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil)
				mockEventBus.EXPECT().Publish("book_snapshot", gomock.Any())
			},
			registerTypes: map[common.MessageType]MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: "",
		},
		{
			name:    "invalid JSON message",
			message: []byte(`invalid json`),
			setupMockStubs: func() {
				// No mocks needed as it should fail before handler
			},
			registerTypes: map[common.MessageType]MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: "failed to parse message",
		},
		{
			name:    "no handler registered",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				// No mocks needed as there's no handler
			},
			registerTypes: map[common.MessageType]MessageHandler{},
			expectedError: "no handler registered for message type",
		},
		{
			name:    "handler error",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				mockHandler.EXPECT().Handle(gomock.Any()).Return(fmt.Errorf("handler error"))
			},
			registerTypes: map[common.MessageType]MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: "handler error for message type book_snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// comment / uncomment to see logs
			logger.SetLevel("trace")

			// Register handlers for this test case
			for msgType, handler := range tt.registerTypes {
				d.RegisterHandler(msgType, handler)
			}

			// Setup mocks
			tt.setupMockStubs()

			// Execute dispatch
			err := d.dispatch(tt.message)

			// Verify error
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}

			// Clear handlers for next test
			d.handlers = make(map[common.MessageType]MessageHandler)
		})
	}
}
