package kafka

import (
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/alejoacosta74/go-logger"
)

// CheckClusterAvailability verifies if the Kafka cluster is available and responsive
func CheckClusterAvailability(brokers []string, timeout time.Duration) error {
	// Create Sarama config with timeout
	config := sarama.NewConfig()
	config.Net.DialTimeout = timeout
	config.Net.ReadTimeout = timeout
	config.Net.WriteTimeout = timeout

	logger.Tracef("Checking Kafka cluster availability with brokers: %v", brokers)
	// Create a client
	client, err := sarama.NewClient(brokers, config)
	if err != nil {
		return fmt.Errorf("failed to create kafka client: %w", err)
	}
	logger.Tracef("Kafka client created successfully")
	defer client.Close()

	// Get broker list
	availableBrokers := client.Brokers()
	if len(availableBrokers) == 0 {
		return fmt.Errorf("no brokers available in the cluster")
	}
	logger.Tracef("Kafka brokers available: %v", len(availableBrokers))

	// Try to connect to each broker
	for _, broker := range availableBrokers {
		err := broker.Open(config)
		if err != nil {
			return fmt.Errorf("failed to connect to broker %s: %w", broker.Addr(), err)
		}
		connected, err := broker.Connected()
		if err != nil {
			return fmt.Errorf("failed to check connection to broker %s: %w", broker.Addr(), err)
		}
		if !connected {
			return fmt.Errorf("broker %s is not connected", broker.Addr())
		}
		logger.Tracef("Broker %s is connected", broker.Addr())
		broker.Close()
	}

	return nil
}
