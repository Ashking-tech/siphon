package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"siphon/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Siphon version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if globals.JSON {
				emitJSON(map[string]any{"ok": true, "version": version.String()})
				return nil
			}
			fmt.Printf("siphon %s\n", version.String())
			return nil
		},
	}
}
