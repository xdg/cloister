package cmd

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
)

var executorCmd = &cobra.Command{
	Use:    "executor",
	Short:  "Manage the host command executor (internal)",
	Hidden: true, // Internal command, not for user invocation
}

var executorRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the executor socket server (internal)",
	Long: `Run the executor socket server for host command execution.

This command is intended to be run as a background process by 'cloister guardian start'
and should not normally be invoked directly by users.

The executor listens on a Unix socket and executes approved commands on the host.
It validates requests using a shared secret (guardian-executor authentication).
Token validation is handled by the guardian before forwarding requests.`,
	Hidden: true,
	RunE:   runExecutor,
}

func init() {
	executorCmd.AddCommand(executorRunCmd)
	rootCmd.AddCommand(executorCmd)
}

// runExecutor starts the executor socket server and blocks until interrupted.
func runExecutor(cmd *cobra.Command, args []string) error {
	// Switch to daemon mode: logs go to file only, not stderr
	clog.SetDaemonMode(true)

	// Get shared secret from environment
	secret := os.Getenv(guardian.SharedSecretEnvVar)
	if secret == "" {
		return fmt.Errorf("shared secret not provided (set %s)", guardian.SharedSecretEnvVar)
	}

	// Create real executor
	realExecutor := executor.NewRealExecutor()

	// Create socket server with TCP mode (for Docker compatibility on macOS)
	// Bind to 127.0.0.1:0 to get a random available port
	// Token validation is handled by the guardian before forwarding requests
	server := executor.NewSocketServer(
		secret,
		realExecutor,
		executor.WithTCPAddr("127.0.0.1:0"),
	)

	// Start the server
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start executor server: %w", err)
	}

	// Get the actual port that was bound
	listenAddr := server.ListenAddr()
	_, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return fmt.Errorf("failed to parse listen address: %w", err)
	}
	port, _ := strconv.Atoi(portStr)

	clog.Info("executor server listening on %s", listenAddr)

	// Save daemon state with TCP port
	state := &executor.DaemonState{
		PID:     os.Getpid(),
		Secret:  secret,
		TCPPort: port,
	}
	if err := executor.SaveDaemonState(state); err != nil {
		clog.Warn("failed to save daemon state: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	clog.Debug("shutting down executor server...")

	// Clean up
	if err := server.Stop(); err != nil {
		clog.Error("error during shutdown: %v", err)
	}

	// Remove daemon state
	if err := executor.RemoveDaemonState(); err != nil {
		clog.Warn("failed to remove daemon state: %v", err)
	}

	clog.Debug("executor server stopped")
	return nil
}
