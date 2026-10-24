// Package cli implements the cobra surface. The same JSON result shape is
// consumed by the KDE wrapper and the GUI.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"siphon/internal/core"
	"siphon/internal/siphonerr"
)

// Globals are flags shared by every command.
type Globals struct {
	JSON           bool
	AllowRoot      bool
	AllowHardlinks bool
	Timeout        time.Duration
}

var globals Globals

// Execute runs the CLI and returns the process exit status.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		var silent *silentError
		if !errors.As(err, &silent) {
			printError(err)
		}
		if errors.Is(err, context.Canceled) {
			return 130
		}
		return siphonerr.ExitCode(err)
	}
	return 0
}

// NewRoot builds the command tree (also used by tests).
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "siphon",
		Short:         "Convert media in place without needing twice the disk space",
		Long:          "Siphon converts media files in place on Linux.\nIt refuses before it touches anything it cannot handle safely.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	pf := root.PersistentFlags()
	pf.BoolVar(&globals.JSON, "json", false, "machine-readable result on stdout")
	pf.BoolVar(&globals.AllowRoot, "allow-root", false, "permit running as UID 0")
	pf.BoolVar(&globals.AllowHardlinks, "allow-hardlinks", false, "permit st_nlink > 1")
	pf.DurationVar(&globals.Timeout, "timeout", time.Hour, "subprocess deadline")

	root.AddCommand(newConvertCmd())
	root.AddCommand(newInspectCmd())
	root.AddCommand(newRecoverCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newVersionCmd())
	return root
}

func printError(err error) {
	if globals.JSON {
		res := &core.Result{
			OK:              false,
			Code:            string(siphonerr.CodeOf(err)),
			Message:         err.Error(),
			SourceUntouched: siphonerr.SourceUntouched(err),
		}
		emitJSON(res)
		return
	}
	fmt.Fprintf(os.Stderr, "siphon: %v\n", err)
}

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// silentError marks an error already rendered by its command.
type silentError struct{ err error }

func (e *silentError) Error() string { return e.err.Error() }
func (e *silentError) Unwrap() error { return e.err }

// isTerminal reports whether f is a character device (a real terminal).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func emitFailure(res *core.Result, err error) error {
	if globals.JSON {
		if res == nil {
			res = &core.Result{
				OK:              false,
				Code:            string(siphonerr.CodeOf(err)),
				Message:         err.Error(),
				SourceUntouched: siphonerr.SourceUntouched(err),
			}
		}
		emitJSON(res)
	} else {
		fmt.Fprintf(os.Stderr, "siphon: %v\n", err)
	}
	return &silentError{err: err}
}
