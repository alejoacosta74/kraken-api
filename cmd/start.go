/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/alejoacosta74/go-logger"
	disp "github.com/alejoacosta74/kraken-api/internal/dispatcher"
	"github.com/alejoacosta74/kraken-api/internal/dispatcher/handlers"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/internal/kafka"
	"github.com/alejoacosta74/kraken-api/internal/metrics"
	"github.com/alejoacosta74/kraken-api/internal/ui"
	"github.com/alejoacosta74/kraken-api/internal/ws"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// startCmd represents the start command for the WebSocket client application.
// It initializes and runs the WebSocket connection to the Kraken API.
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the ws client",
	Long:  `Start the ws client`,
	Run:   runStart,
	Args:  cobra.ExactArgs(1), // the ws url is the only argument
}

// init initializes the command-line flags and binds them to viper configuration.
// This function is automatically called by Cobra when the package is initialized.
func init() {
	rootCmd.AddCommand(startCmd)
	// Add trading pair flag with default value "ETH/USD"
	startCmd.Flags().String("pair", "ETH/USD", "Trading pair to subscribe to")
	// Bind the flag to viper for configuration management
	viper.BindPFlag("tradingpair", startCmd.Flags().Lookup("pair"))
}

// runStart is the main entry point for the WebSocket client application.
// It performs the following tasks:
// 1. Sets up context for graceful shutdown
// 2. Initializes communication channels
// 3. Creates and configures the WebSocket client
// 4. Starts error and message handling goroutines
// 5. Runs the main client loop
//
// Parameters:
//   - cmd: The Cobra command being executed
//   - args: Command line arguments (expects WebSocket URL as first argument)
func runStart(cmd *cobra.Command, args []string) {
	wsUrl := args[0]
	pair := viper.GetString("tradingpair")

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	go handleSignals(cancel)

	// Initialize channels for internal communication
	// msgChan: carries WebSocket messages from reader to handlers
	// errChan: carries errors from various components
	msgChan := make(chan []byte, 100) // Buffer size 100 to prevent blocking
	errChan := make(chan error, 10)   // Buffer size 10 for errors

	// Create event bus
	eventBus := events.NewEventBus()

	// Create dispatcher
	dispatcher := disp.NewDispatcher(msgChan, errChan, eventBus)

	// Create Kafka producer
	kafkaProducer, err := kafka.NewProducer([]string{"192.168.5.142:9092"})
	if err != nil {
		logger.Fatal("Failed to create Kafka producer:", err)
	}
	defer kafkaProducer.Close()

	// Create and register handlers
	baseHandler := handlers.NewBaseHandler(kafkaProducer, "book_snapshots")
	snapshotHandler := handlers.NewBookSnapshotHandler(baseHandler)
	dispatcher.RegisterHandler(disp.TypeBookSnapshot, snapshotHandler)

	// Register debug handler for all message types
	debugHandler := handlers.NewDebugHandler()
	dispatcher.RegisterHandler(disp.TypeBookUpdate, debugHandler)
	dispatcher.RegisterHandler(disp.TypeHeartbeat, debugHandler)
	dispatcher.RegisterHandler(disp.TypeSystem, debugHandler)

	// Initialize metrics collector
	metricsCollector := metrics.NewMetricsCollector(eventBus, metrics.Config{
		MetricsAddr: ":2112",
	})

	// Start metrics collector
	go func() {
		if err := metricsCollector.Start(ctx, metrics.Config{MetricsAddr: ":2112"}); err != nil {
			logger.Error("Metrics collector error:", err)
		}
	}()

	// Initialize UI updater
	uiUpdater := ui.NewUIUpdater(eventBus)
	go uiUpdater.Start()

	// Start dispatcher
	go dispatcher.Run(ctx)

	// Create WebSocket client
	wsClient := ws.NewWebSocketClient(
		wsUrl,
		ws.WithTradingPair(pair),
		ws.WithBuffers(msgChan, errChan),
	)

	// Start error handling goroutine
	// This will be enhanced with more sophisticated error handling in future iterations
	go func() {
		for err := range errChan {
			logger.Error("WebSocket error:", err)
		}
	}()

	// Start message handling goroutine
	// This is a temporary implementation that will be replaced by the dispatcher
	go func() {
		for msg := range msgChan {
			logger.Debug("Received message:", string(msg))
		}
	}()

	// Run the client and handle any fatal errors
	if err := wsClient.Run(ctx); err != nil {
		logger.Error("WebSocket client error:", err)
	}

	logger.Info("Client shutdown complete")
}
