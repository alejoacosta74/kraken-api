/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"sync"
	"websocket-client/client"

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

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	// Capture system signals for graceful shutdown
	go handleSignals(cancel)

	wsClient := client.NewWebSocketClient(wsUrl, client.WithTradingPair(pair))

	wg.Add(1)
	go func() {
		defer wg.Done()
		wsClient.Run(ctx)
	}()

	// Wait for and process the book snapshot
	wg.Add(1)
	go func() {
		defer wg.Done()
		snapshot, err := wsClient.GetBookSnapshot(ctx)
		if err != nil {
			logger.Errorf("Failed to get book snapshot: %v", err)
			cancel() // Cancel context on error
			return
		}

		fmt.Println(snapshot.PrettyPrint())
	}()

	wg.Wait()
	logger.Info("Client shutdown")
}
