package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"siphon/internal/convert"
	"siphon/internal/progress"
)

func newConvertCmd() *cobra.Command {
	var flags conversionFlags
	cmd := &cobra.Command{
		Use:   "convert <source>",
		Short: "Convert a media file in place",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			job, err := flags.build(args[0])
			if err != nil {
				return emitFailure(nil, err)
			}
			if flags.dryRun {
				return runInspect(cmd, job)
			}

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

			res, err := convert.Run(cmd.Context(), job, sink)
			if err != nil {
				return emitFailure(res, err)
			}
			if globals.JSON {
				emitJSON(res)
				return nil
			}
			fmt.Printf("converted %s → %s (%s, %s)\n",
				job.SourcePath, job.TargetPath, job.Mode, job.Pair.OutputKindFor(job.Mode))
			return nil
		},
	}
	flags.bind(cmd)
	return cmd
}
