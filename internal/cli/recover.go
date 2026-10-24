package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"siphon/internal/progress"
	"siphon/internal/recover"
)

func newRecoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recover <source>",
		Short: "Resume or salvage a crashed job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]

			var sink progress.Sink
			switch {
			case globals.JSON:
				sink = progress.NewJSON(os.Stderr)
			case isTerminal(os.Stderr):
				sink = progress.NewTerminal(os.Stderr)
			default:
				sink = progress.NewPlain(os.Stderr)
			}
			defer sink.Close()

			res, journalPath, err := recover.Run(cmd.Context(), source, sink)
			if err != nil {
				return emitFailure(res, err)
			}
			if globals.JSON {
				emitJSON(res)
				return nil
			}
			fmt.Printf("recovered %s (journal %s)\n",
				filepath.Clean(source), filepath.Clean(journalPath))
			return nil
		},
	}
}
