/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"sync"
	"websocket-client/client"
	"websocket-client/kraken"

	"github.com/alejoacosta74/go-logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the ws client",
	Long:  `Start the ws client`,
	Run:   runStart,
	Args:  cobra.ExactArgs(1), // the ws url is the only argument

}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().String("pair", "ETH/USD", "Trading pair to subscribe to")
	viper.BindPFlag("tradingpair", startCmd.Flags().Lookup("pair"))

}

func runStart(cmd *cobra.Command, args []string) {
	wsUrl := args[0]
	pair := viper.GetString("tradingpair")

	ctx, cancel := context.WithCancel(context.Background())

	// handle interrupt signals to cancel the context and shutdown the client
	go handleSignals(cancel)

	wg := &sync.WaitGroup{}

	// Buffered channels to prevent blocking during shutdown
	snapshotChan := make(chan client.Event, 100)
	updateChan := make(chan client.Event, 100)

	wsClient := client.NewWebSocketClient(wsUrl, client.WithTradingPair(pair))

	// Create a done channel for coordinating shutdown
	done := make(chan struct{})

	wsClient.Subscribe(client.EventBookSnapshot, func(e client.Event) {
		select {
		case <-done:
			logger.Debug("Done channel received, not sending BookSnapshot event")
			return
		default:
			snapshotChan <- e
		}
	})

	wsClient.Subscribe(client.EventBookUpdate, func(e client.Event) {
		select {
		case <-done:
			logger.Debug("Done channel received, not sending BookUpdate event")
			return
		default:
			updateChan <- e
		}
	})

	// Handle events
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)         // Signal event handlers to stop
		defer close(snapshotChan) // Safe to close since handlers will stop
		defer close(updateChan)   // Safe to close since handlers will stop

		for {
			select {
			case <-ctx.Done():
				logger.Debug("Context cancelled, exiting event handler")
				return
			case event := <-snapshotChan:
				if snapshot, ok := event.Data.(kraken.BookSnapshot); ok {
					fmt.Println("Snapshot received:", snapshot.PrettyPrint())
				}
			case event := <-updateChan:
				if update, ok := event.Data.(kraken.SnapshotUpdate); ok {
					fmt.Println("Update received:", update.Data[0].Symbol)
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		wsClient.Run(ctx)
	}()

	wg.Wait()
	logger.Info("Client shutdown complete")
}
