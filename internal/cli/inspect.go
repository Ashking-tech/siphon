package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"siphon/internal/convert"
	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/preflight"
	"siphon/internal/siphonerr"
)

func newInspectCmd() *cobra.Command {
	var flags conversionFlags
	cmd := &cobra.Command{
		Use:   "inspect <source>",
		Short: "Dry run: show the plan or the refusal, change nothing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			if flags.to == "" {
				return inspectTargets(source)
			}
			job, err := flags.build(source)
			if err != nil {
				return emitFailure(nil, err)
			}
			return runInspect(cmd, job)
		},
	}
	flags.bind(cmd)
	return cmd
}

func runInspect(cmd *cobra.Command, job *core.Job) error {
	plan, err := convert.Inspect(cmd.Context(), job)
	if err != nil {
		res := &core.Result{
			OK:              false,
			Code:            string(siphonerr.CodeOf(err)),
			Message:         siphonerr.MessageOf(err),
			Plan:            plan,
			Hints:           siphonerr.HintsOf(err),
			SourceUntouched: true,
		}
		if !globals.JSON && plan != nil {
			fmt.Fprint(os.Stdout, preflight.FormatPlan(plan))
		}
		return emitFailure(res, err)
	}
	res := &core.Result{OK: true, Plan: plan, SourceUntouched: true}
	if globals.JSON {
		emitJSON(res)
		return nil
	}
	fmt.Fprint(os.Stdout, preflight.FormatPlan(plan))
	return nil
}

func inspectTargets(source string) error {
	src, err := formats.FromPath(source)
	if err != nil {
		return emitFailure(nil, err)
	}
	targets := formats.SupportedTargets(src)
	if len(targets) == 0 {
		return emitFailure(nil, siphonerr.New(siphonerr.FormatUnsupported,
			"no targets are supported for %s", src))
	}
	if globals.JSON {
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.String()
		}
		emitJSON(map[string]any{
			"ok":            true,
			"source":        source,
			"source_format": src.String(),
			"targets":       names,
		})
		return nil
	}
	fmt.Printf("%s (%s)\n  targets: ", source, src)
	for i, t := range targets {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(t)
	}
	fmt.Println()
	return nil
}
