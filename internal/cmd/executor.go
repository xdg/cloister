package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/token"
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
It validates requests using a shared secret and token-based authentication.`,
	Hidden: true,
	RunE:   runExecutor,
}

func init() {
	executorCmd.AddCommand(executorRunCmd)
	rootCmd.AddCommand(executorCmd)
}

// runExecutor starts the executor socket server and blocks until interrupted.
func runExecutor(cmd *cobra.Command, args []string) error {
	// Get shared secret from environment
	secret := os.Getenv(guardian.SharedSecretEnvVar)
	if secret == "" {
		return fmt.Errorf("shared secret not provided (set %s)", guardian.SharedSecretEnvVar)
	}

	// Get socket path
	socketPath, err := guardian.HostSocketPath()
	if err != nil {
		return fmt.Errorf("failed to get socket path: %w", err)
	}

	// Create token registry and load persisted tokens
	registry := token.NewRegistry()
	tokenDir, err := token.DefaultTokenDir()
	if err != nil {
		log.Printf("Warning: failed to get token directory: %v", err)
	} else {
		store, err := token.NewStore(tokenDir)
		if err != nil {
			log.Printf("Warning: failed to open token store: %v", err)
		} else {
			tokens, err := store.Load()
			if err != nil {
				log.Printf("Warning: failed to load tokens: %v", err)
			} else {
				for tok, info := range tokens {
					registry.RegisterFull(tok, info.CloisterName, info.ProjectName, info.WorktreePath)
				}
				if len(tokens) > 0 {
					log.Printf("Loaded %d tokens from disk", len(tokens))
				}
			}
		}
	}

	// Create real executor
	realExecutor := executor.NewRealExecutor()

	// Create token validator that looks up in the registry
	tokenValidator := func(tok string) (string, error) {
		info, ok := registry.Lookup(tok)
		if !ok {
			return "", executor.ErrInvalidToken
		}
		return info.WorktreePath, nil
	}

	// Create workdir validator that compares paths
	workdirValidator := func(requestedWorkdir, registeredWorktree string) error {
		// Require exact match for security
		if requestedWorkdir != registeredWorktree {
			return executor.ErrWorkdirMismatch
		}
		return nil
	}

	// Create socket server with validators
	server := executor.NewSocketServer(
		secret,
		realExecutor,
		executor.WithSocketPath(socketPath),
		executor.WithTokenValidator(tokenValidator),
		executor.WithWorkdirValidator(workdirValidator),
	)

	// Start the server
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start executor socket server: %w", err)
	}

	log.Printf("Executor socket server listening on %s", socketPath)

	// Save daemon state
	state := &executor.DaemonState{
		PID:        os.Getpid(),
		Secret:     secret,
		SocketPath: socketPath,
	}
	if err := executor.SaveDaemonState(state); err != nil {
		log.Printf("Warning: failed to save daemon state: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down executor socket server...")

	// Clean up
	if err := server.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	// Remove daemon state
	if err := executor.RemoveDaemonState(); err != nil {
		log.Printf("Warning: failed to remove daemon state: %v", err)
	}

	log.Println("Executor socket server stopped")
	return nil
}
