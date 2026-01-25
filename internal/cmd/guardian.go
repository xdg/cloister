package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/token"
)

var guardianCmd = &cobra.Command{
	Use:   "guardian",
	Short: "Manage the guardian proxy service",
	Long: `Manage the guardian proxy service that provides allowlist-enforced network
access for cloister containers.

The guardian runs as a separate container and provides:
- HTTP CONNECT proxy with domain allowlist
- Per-cloister token authentication
- Host command execution approval (future)`,
}

var guardianStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the guardian service",
	Long:  `Start the guardian container if not already running.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian start: not implemented")
		return nil
	},
}

var guardianStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the guardian service",
	Long: `Stop the guardian container.

Warns if there are running cloister containers that depend on the guardian.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian stop: not implemented")
		return nil
	},
}

var guardianStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show guardian service status",
	Long:  `Show guardian status including uptime and active token count.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloister guardian status: not implemented")
		return nil
	},
}

var guardianRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the guardian proxy server (internal)",
	Long: `Run the guardian proxy and API servers.

This command is intended to be run inside the guardian container and should
not normally be invoked directly by users.

The proxy server (port 3128):
- Listens for HTTP CONNECT requests
- Enforces the domain allowlist
- Validates per-cloister authentication tokens

The API server (port 9997):
- Provides token registration/revocation endpoints
- Used by the CLI to manage tokens

Both servers block until interrupted (SIGINT/SIGTERM).`,
	RunE: runGuardianProxy,
}

func init() {
	guardianCmd.AddCommand(guardianStartCmd)
	guardianCmd.AddCommand(guardianStopCmd)
	guardianCmd.AddCommand(guardianStatusCmd)
	guardianCmd.AddCommand(guardianRunCmd)
	rootCmd.AddCommand(guardianCmd)
}

// runGuardianProxy starts the proxy and API servers and blocks until interrupted.
func runGuardianProxy(cmd *cobra.Command, args []string) error {
	// Create an in-memory token registry
	// Note: This means tokens are lost on container restart - persistence
	// will be added in a later phase.
	registry := token.NewRegistry()

	// Create the proxy server
	proxyAddr := fmt.Sprintf(":%d", guardian.DefaultProxyPort)
	proxy := guardian.NewProxyServer(proxyAddr)
	proxy.TokenValidator = registry
	proxy.Logger = log.New(os.Stderr, "[guardian] ", log.LstdFlags)

	// Create the API server
	apiAddr := fmt.Sprintf(":%d", guardian.DefaultAPIPort)
	api := guardian.NewAPIServer(apiAddr, registry)

	// Start the proxy server
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Printf("Guardian proxy server listening on %s", proxy.ListenAddr())

	// Start the API server
	if err := api.Start(); err != nil {
		// Clean up proxy server on failure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Stop(ctx)
		return fmt.Errorf("failed to start API server: %w", err)
	}

	log.Printf("Guardian API server listening on %s", api.ListenAddr())

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down guardian servers...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop both servers
	var proxyErr, apiErr error
	proxyErr = proxy.Stop(ctx)
	apiErr = api.Stop(ctx)

	if proxyErr != nil {
		return fmt.Errorf("error during proxy shutdown: %w", proxyErr)
	}
	if apiErr != nil {
		return fmt.Errorf("error during API shutdown: %w", apiErr)
	}

	log.Println("Guardian servers stopped")
	return nil
}
