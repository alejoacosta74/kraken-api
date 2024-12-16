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
	"github.com/alejoacosta74/kraken-api/internal/kafka"
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

	// Get Kafka brokers from viper
	kafkaBrokers := viper.GetStringSlice("kafka.cluster.addresses")

	// Check Kafka availability with a reasonable timeout
	if err := kafka.CheckClusterAvailability(kafkaBrokers, 10*time.Second); err != nil {
		logger.Fatalf("kafka cluster is not available: %v", err)
	}

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

	// create producer pool
	producerPool := kafka.NewProducerPool(ctx, viper.GetInt("kafka.producer.pool.size"))

	// Create channels for WebSocket communication
	msgChan := make(chan []byte, 100)
	dispatcherErrChan := make(chan error, 10)
	dispatcherDoneChan := make(chan struct{})

	dispatcherCfg := disp.DispatcherConfig{
		Ctx:          ctx,
		MsgChan:      msgChan,
		ErrChan:      dispatcherErrChan,
		DoneChan:     dispatcherDoneChan,
		ProducerPool: producerPool,
		EventBus:     eventBus,
	}

	// Create and configure dispatcher
	dispatcher := disp.NewDispatcher(dispatcherCfg)

	// Create base handler with producer pool
	baseHandler := handlers.NewBaseHandler(ctx, producerPool, "kraken_book")

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

	// Create channels for component-specific shutdown signals
	type componentStatus struct {
		name string
		err  error
	}

	shutdownStatuses := make(chan componentStatus, 4) // Buffer for all components

	// Start metrics server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := metricsServer.Start(ctx); err != nil {
			shutdownStatuses <- componentStatus{"metrics_server", err}
			logger.Error("Metrics server error:", err)
		}
		shutdownStatuses <- componentStatus{"metrics_server", nil}
	}()

	// Start metrics recorder
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := metricsRecorder.Start(ctx); err != nil {
			shutdownStatuses <- componentStatus{"metrics_recorder", err}
			logger.Error("Metrics recorder error:", err)
		}
		shutdownStatuses <- componentStatus{"metrics_recorder", nil}
	}()

	// Start dispatcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		dispatcher.Run(ctx)
		shutdownStatuses <- componentStatus{"dispatcher", nil}
	}()

	// Start WebSocket client
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := wsClient.Run(); err != nil {
			shutdownStatuses <- componentStatus{"websocket_client", err}
			logger.Fatalf("WebSocket client error: %v", err)
			cancel() // Trigger shutdown if WebSocket fails
			return
		}
		<-wsDoneChan
		shutdownStatuses <- componentStatus{"websocket_client", nil}
	}()

	// go routine to log all the errors received from the error channels
	go func() {
		for {
			select {
			case err, ok := <-dispatcherErrChan:
				if !ok {
					return
				}
				logger.Error("Dispatcher error:", err)
			case err, ok := <-wsErrChan:
				if !ok {
					return
				}
				logger.Error("WebSocket client error:", err)
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	logger.Info("Shutdown initiated")

	// Wait for components with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()

	// Create a channel to signal when all components are done
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	// Track shutdown status of components
	componentStatuses := make(map[string]error)
	componentsRunning := 4 // Number of components we're waiting for

	// Wait for either all components to shut down or timeout
	for {
		select {
		case status := <-shutdownStatuses:
			componentStatuses[status.name] = status.err
			componentsRunning--
			logger.Infof("Component %s shutdown complete", status.name)
			if componentsRunning == 0 {
				logger.Info("All components shut down successfully")
				return
			}

		case <-allDone:
			logger.Info("All components shut down cleanly")
			return

		case <-shutdownCtx.Done():
			logger.Error("Shutdown timeout reached - forcing exit")
			// Log status of components that haven't shut down
			for _, component := range []string{"metrics_server", "metrics_recorder", "dispatcher", "websocket_client"} {
				if _, ok := componentStatuses[component]; !ok {
					logger.Errorf("Component %s failed to shut down in time", component)
				}
			}
			// Force exit after logging
			os.Exit(1)
		}
	}
}
