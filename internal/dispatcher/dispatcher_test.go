package dispatcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alejoacosta74/go-logger"
	"github.com/alejoacosta74/kraken-api/internal/common"
	handlers "github.com/alejoacosta74/kraken-api/internal/dispatcher/handlers"
	handlersmocks "github.com/alejoacosta74/kraken-api/internal/dispatcher/handlers/mocks"
	eventmocks "github.com/alejoacosta74/kraken-api/internal/events/mocks"
	kafkamocks "github.com/alejoacosta74/kraken-api/internal/kafka/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestDispatcher_Run(t *testing.T) {
	// comment / uncomment to see logs
	// logger.SetLevel("trace")
	logger.NullOutput()

	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockProducerPool := kafkamocks.NewMockProducerPool(ctrl)
	mockEventBus := eventmocks.NewMockBus(ctrl)
	mockHandler := handlersmocks.NewMockMessageHandler(ctrl)

	// Test cases
	tests := []struct {
		name           string
		message        [][]byte
		setupMockStubs func()
		expectedError  bool
	}{
		{
			name:    "successful unique message processing",
			message: createTestUpdateMessages(1),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil).Times(1)
				mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(1)
			},
			expectedError: false,
		},
		{
			name:    "successful multiple message processing",
			message: createTestUpdateMessages(10),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil).Times(10)
				mockEventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(10)
			},
			expectedError: false,
		},
		{
			name:    "handler error",
			message: createTestUpdateMessages(1),
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
				mockHandler.EXPECT().Handle(gomock.Any()).Return(assert.AnError).Times(1)
			},
			expectedError: true,
		},
		{
			name:    "malformed JSON message",
			message: [][]byte{[]byte(`malformed JSON`)},
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
			},
			expectedError: true,
		},
		{
			name:    "empty message",
			message: [][]byte{[]byte(`""`)},
			setupMockStubs: func() {
				mockProducerPool.EXPECT().Start().Times(1)
				mockProducerPool.EXPECT().Stop().Times(1)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Create channels
			msgChan := make(chan []byte, 1) // channel to receive messages
			errChan := make(chan error, 1)  // channel to receive errors
			doneChan := make(chan struct{}) // channel to wait for dispatcher to finish

			// Create context
			ctx, cancel := context.WithCancel(context.Background())
			defer goleak.VerifyNone(t)

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
			dispatcher.RegisterHandler(common.TypeBookUpdate, mockHandler)

			// Setup mocks
			tt.setupMockStubs()

			// Start dispatcher in goroutine
			go dispatcher.Run()

			// Send test message
			for _, msg := range tt.message {
				select {
				case msgChan <- msg:
					// Message sent successfully
				case <-time.After(time.Second):
					t.Fatal("timeout sending message")
				}
			}

			// Wait for all messages to be processed
			time.Sleep(time.Duration(len(tt.message)) * 100 * time.Millisecond)

			// Check for errors
			if tt.expectedError {
				select {
				case err := <-errChan:
					assert.Error(t, err)
				case <-time.After(6 * time.Second):
					t.Error("timeout waiting for error")
				}
			}

			// Cancel context
			cancel()
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
	mockProducerPool := kafkamocks.NewMockProducerPool(ctrl)
	mockEventBus := eventmocks.NewMockBus(ctrl)
	mockHandler := handlersmocks.NewMockMessageHandler(ctrl)

	// Create test dispatcher
	errChan := make(chan error)
	cfg := DispatcherConfig{
		MsgChan:      make(chan []byte),
		ErrChan:      errChan,
		DoneChan:     make(chan struct{}),
		EventBus:     mockEventBus,
		ProducerPool: mockProducerPool,
		Ctx:          context.Background(),
	}
	d := NewDispatcher(cfg)

	tests := []struct {
		name             string
		message          []byte
		setupMockStubs   func()
		registerHandlers map[common.MessageType]handlers.MessageHandler
		expectedError    bool
	}{
		{
			name:    "successful book snapshot dispatch",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {"symbol": "XBT/USD"}}`),
			setupMockStubs: func() {
				mockHandler.EXPECT().Handle(gomock.Any()).Return(nil)
				mockEventBus.EXPECT().Publish(common.TypeBookSnapshot, gomock.Any())
			},
			registerHandlers: map[common.MessageType]handlers.MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: false,
		},
		{
			name:    "invalid JSON message",
			message: []byte(`invalid json`),
			setupMockStubs: func() {
				// No mocks needed as it should fail before handler
			},
			registerHandlers: map[common.MessageType]handlers.MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: true,
		},
		{
			name:    "no handler registered",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				// No mocks needed as there's no handler
			},
			registerHandlers: map[common.MessageType]handlers.MessageHandler{},
			expectedError:    true,
		},
		{
			name:    "handler error",
			message: []byte(`{"channel": "book", "type": "snapshot", "data": {}}`),
			setupMockStubs: func() {
				mockHandler.EXPECT().Handle(gomock.Any()).Return(fmt.Errorf("handler error"))
			},
			registerHandlers: map[common.MessageType]handlers.MessageHandler{
				common.TypeBookSnapshot: mockHandler,
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Register handlers for this test case
			for msgType, handler := range tt.registerHandlers {
				d.RegisterHandler(msgType, handler)
			}

			// Setup mocks
			tt.setupMockStubs()

			// Execute dispatch
			err := d.dispatch(tt.message)

			// Verify error
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Clear handlers for next test
			d.handlers = make(map[common.MessageType]handlers.MessageHandler)
		})
	}
}

// createTestUpdateMessages creates a slice of test orderbook update messages
func createTestUpdateMessages(qty int) [][]byte {
	messages := make([][]byte, qty)
	for i := 0; i < qty; i++ {
		messages[i] = []byte(`{"channel": "book", "type": "update", "data": {"symbol": "XBT/USD"}}`)
	}
	return messages
}
