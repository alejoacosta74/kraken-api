//go:generate mockgen -destination=mock_interfaces.go -package=mocks github.com/alejoacosta74/kraken-api/internal/kafka ProducerPool
//go:generate mockgen -source=../dispatcher.go -destination=mock_message_handler.go -package=mocks
//go:generate mockgen -destination=mock_event_bus.go -package=mocks github.com/alejoacosta74/kraken-api/internal/events Bus

package mocks
