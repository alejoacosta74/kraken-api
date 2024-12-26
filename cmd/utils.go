package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	_ "net/http/pprof"
)

// handleSignals listens for OS signals to cancel the context
func handleSignals(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	cancel()
}
