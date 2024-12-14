/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alejoacosta74/go-logger"
	disp "github.com/alejoacosta74/kraken-api/internal/dispatcher"
	"github.com/alejoacosta74/kraken-api/internal/dispatcher/handlers"
	"github.com/alejoacosta74/kraken-api/internal/events"
	"github.com/alejoacosta74/kraken-api/internal/metrics"
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
	// Add metrics configuration flags
	startCmd.Flags().String("metrics-addr", ":2112", "Address to serve metrics on")
	startCmd.Flags().Int("metrics-buffer", 100, "Buffer size for metrics channels")
	// Add kafka configuration flags
	startCmd.Flags().StringSlice("kafka-cluster-addresses", []string{"192.168.4.248:9092"}, "Kafka cluster addresses")
	// Bind all flags to viper
	viper.BindPFlag("tradingpair", startCmd.Flags().Lookup("pair"))
	viper.BindPFlag("metrics.addr", startCmd.Flags().Lookup("metrics-addr"))
	viper.BindPFlag("metrics.buffer", startCmd.Flags().Lookup("metrics-buffer"))
	viper.BindPFlag("kafka.cluster.addresses", startCmd.Flags().Lookup("kafka-cluster-addresses"))
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

	// Set up signal handling - simplified
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel() // Single point of shutdown initiation
	}()

	// Create components
	eventBus := events.NewEventBus()
	metricsServer := metrics.NewMetricsServer(viper.GetString("metrics.addr"))
	metricsRecorder := metrics.NewMetricsRecorder(ctx, eventBus)

	// Create channels for WebSocket communication
	msgChan := make(chan []byte, 100)
	dispatcherErrChan := make(chan error, 10)

	// Create and configure dispatcher
	dispatcher := disp.NewDispatcher(ctx, msgChan, dispatcherErrChan, eventBus)

	// Create base handler with producer pool
	baseHandler := handlers.NewBaseHandler(dispatcher.GetProducerPool(), "kraken_book")

	// Create handlers
	debugHandler := handlers.NewDebugHandler()
	snapshotHandler := handlers.NewBookSnapshotHandler(baseHandler)
	updateHandler := handlers.NewBookUpdateHandler(baseHandler)

	// Register handlers with dispatcher
	dispatcher.RegisterHandler(disp.TypeBookSnapshot, snapshotHandler)
	dispatcher.RegisterHandler(disp.TypeBookUpdate, updateHandler)
	dispatcher.RegisterHandler(disp.TypeHeartbeat, debugHandler)
	dispatcher.RegisterHandler(disp.TypeSystem, debugHandler)
	dispatcher.RegisterHandler("subscription_response", debugHandler)

	// Create WebSocket client
	wsErrChan := make(chan error, 10)
	wsDoneChan := make(chan struct{})
	clientCfg := ws.ClientConfig{
		Ctx:      ctx,
		Url:      wsUrl,
		MsgChan:  msgChan,
		ErrChan:  wsErrChan,
		DoneChan: wsDoneChan,
		Opts:     []ws.Option{ws.WithTradingPair(pair)},
	}
	wsClient := ws.NewWebSocketClient(clientCfg)

	// Start components with a single WaitGroup
	var wg sync.WaitGroup

	// Start metrics server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := metricsServer.Start(ctx); err != nil {
			logger.Error("Metrics server error:", err)
		}
	}()

	// Start metrics recorder
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := metricsRecorder.Start(ctx); err != nil {
			logger.Error("Metrics recorder error:", err)
		}
	}()

	// Start dispatcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		dispatcher.Run(ctx)
	}()

	// Start WebSocket client
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := wsClient.Run(); err != nil {
			logger.Fatalf("WebSocket client error: %v", err)
		}
		<-wsDoneChan
	}()

	// Wait for context cancellation
	<-ctx.Done()
	logger.Info("Shutdown initiated")

	// Wait for components with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()

	// Create channels to track individual component shutdown
	dispatcherDone := make(chan struct{})
	// metricsDone := make(chan struct{})
	// wsClientDone := make(chan struct{})

	// Wait for dispatcher with timeout
	go func() {
		wg.Wait()
		close(dispatcherDone)
	}()

	// Wait for each component with individual timeouts
	select {
	case <-dispatcherDone:
		logger.Info("All components shut down cleanly")
	case <-shutdownCtx.Done():
		logger.Error("Shutdown timeout - checking individual components")
		// Log which components are still running
		select {
		case <-dispatcherDone:
			logger.Info("Dispatcher shut down")
		default:
			logger.Error("Dispatcher failed to shut down")
		}
		// ... similar checks for other components
	}
}
