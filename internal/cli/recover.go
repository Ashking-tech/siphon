package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"siphon/internal/journal"
	"siphon/internal/preflight"
	"siphon/internal/siphonerr"
)

func newRecoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recover <source>",
		Short: "Resume or salvage a crashed job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			dir := filepath.Dir(source)
			stem := preflight.Stem(source)

			path, err := journal.FindOne(dir, stem)
			if err != nil {
				return emitFailure(nil, err)
			}
			store, j, err := journal.Open(path)
			if err != nil {
				return emitFailure(nil, err)
			}
			defer store.Close()

			if err := journal.VerifySource(j, source); err != nil {
				return emitFailure(nil, err)
			}

			done := 0
			for _, seg := range j.Segments {
				if seg.State == "done" {
					done++
				}
			}

			if globals.JSON {
				emitJSON(map[string]any{
					"ok":               false,
					"code":             string(siphonerr.RecoveryAvailable),
					"message":          "recovery is available but this build has no recovery driver yet",
					"journal":          path,
					"phase":            j.Phase,
					"engine":           j.Job.Engine,
					"mode":             j.Job.Mode,
					"segments":         len(j.Segments),
					"segments_done":    done,
					"tail_cursor":      j.Cursors.TailCursor,
					"out_cursor":       j.Cursors.OutCursor,
					"source_untouched": false,
				})
				return &silentError{err: siphonerr.New(siphonerr.RecoveryAvailable,
					"recovery is available; no recovery driver in this build")}
			}

			fmt.Printf("journal      %s\n", path)
			fmt.Printf("engine       %s (%s)\n", j.Job.Engine, j.Job.Mode)
			fmt.Printf("phase        %s\n", j.Phase)
			fmt.Printf("segments     %d (%d done)\n", len(j.Segments), done)
			fmt.Printf("tail cursor  %d\n", j.Cursors.TailCursor)
			fmt.Printf("out cursor   %d\n", j.Cursors.OutCursor)

			err = siphonerr.New(siphonerr.RecoveryAvailable,
				"recovery is available but this build has no recovery driver yet")
			return emitFailure(nil, err)
		},
	}
}
