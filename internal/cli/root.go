// Package cli implements the cobra surface. The same JSON result shape is
// consumed by the KDE wrapper and the GUI.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	hints := siphonerr.HintsOf(err)
	if globals.JSON {
		res := &core.Result{
			OK:              false,
			Code:            string(siphonerr.CodeOf(err)),
			Message:         siphonerr.MessageOf(err),
			Hints:           hints,
			SourceUntouched: siphonerr.SourceUntouched(err),
		}
		emitJSON(res)
		return
	}
	fmt.Fprintf(os.Stderr, "siphon: %v\n", err)
	printHints(os.Stderr, hints)
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
	hints := siphonerr.HintsOf(err)
	if globals.JSON {
		if res == nil {
			res = &core.Result{
				OK:              false,
				Code:            string(siphonerr.CodeOf(err)),
				Message:         siphonerr.MessageOf(err),
				Hints:           hints,
				SourceUntouched: siphonerr.SourceUntouched(err),
			}
		} else if len(res.Hints) == 0 {
			res.Hints = hints
		}
		emitJSON(res)
	} else {
		fmt.Fprintf(os.Stderr, "siphon: %v\n", err)
		printHints(os.Stderr, hints)
	}
	return &silentError{err: err}
}

// printHints renders the boolean retry hints in a stable order. The KDE
// wrapper never parses this prose; it uses the JSON booleans.
func printHints(w io.Writer, hints map[string]bool) {
	if len(hints) == 0 {
		return
	}
	for _, name := range []string{
		"transcode_required", "allow_nonstandard", "drop_incompatible",
		"drop_metadata", "bg_required", "audio_track", "no_audio",
	} {
		if !hints[name] {
			continue
		}
		fmt.Fprintf(w, "siphon: hint: %s\n", hintText(name))
	}
}

func hintText(name string) string {
	switch name {
	case "transcode_required":
		return "rerun with --transcode to accept re-encoding"
	case "allow_nonstandard":
		return "rerun with --allow-nonstandard to accept a nonstandard remux, or --transcode"
	case "drop_incompatible":
		return "rerun with --drop-incompatible to skip the incompatible stream"
	case "drop_metadata":
		return "rerun with --drop-metadata to accept the metadata loss"
	case "bg_required":
		return "rerun with --bg <colour> to flatten transparency"
	case "audio_track":
		return "choose a valid zero-based --audio-track ordinal"
	case "no_audio":
		return "the source has no audio track to extract"
	}
	return name
}
