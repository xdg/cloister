package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xdg/cloister/internal/cloister"
)

var pathCmd = &cobra.Command{
	Use:   "path [cloister-name]",
	Short: "Print the host path for a cloister",
	Long: `Print the host directory path for a cloister.

If no cloister name is provided, uses the cloister detected from the current
working directory.

The output is a bare path with no decoration, suitable for use in shell
scripting, e.g.:

    cd $(cloister path my-project)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPath,
}

func init() {
	rootCmd.AddCommand(pathCmd)
}

func runPath(_ *cobra.Command, args []string) error {
	var cloisterName string

	if len(args) > 0 {
		cloisterName = args[0]
	} else {
		name, err := cloister.DetectName()
		if err != nil {
			if gitErr := gitDetectionErrorWithHint(err, "specify cloister name or run from within a git project"); gitErr != nil {
				return gitErr
			}
			return fmt.Errorf("failed to detect cloister name: %w", err)
		}
		cloisterName = name
	}

	reg, err := cloister.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load cloister registry: %w", err)
	}

	entry := reg.FindByName(cloisterName)
	if entry == nil {
		return fmt.Errorf("cloister %q not found in registry", cloisterName)
	}

	// Write directly to os.Stdout (not term.Println) so the output is
	// not suppressed by --silent. This command is designed for scripting:
	//     cd $(cloister path my-project)
	fmt.Fprintln(os.Stdout, entry.HostPath)
	return nil
}
