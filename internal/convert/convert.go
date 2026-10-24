// Package convert is the orchestrator: pre-flight, engine dispatch, and
// uniform result/error shaping for the CLI and the GUI.
package convert

import (
	"context"

	"siphon/internal/core"
	"siphon/internal/engine"
	_ "siphon/internal/engines" // registers the concrete engines
	"siphon/internal/preflight"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Inspect runs only the gate and returns the plan.
func Inspect(ctx context.Context, job *core.Job) (*core.Plan, error) {
	return preflight.Run(ctx, job)
}

// Run executes a conversion. The returned Result is always safe to serialise,
// even when err is non-nil.
func Run(ctx context.Context, job *core.Job, sink progress.Sink) (*core.Result, error) {
	plan, err := preflight.Run(ctx, job)
	if err != nil {
		return resultForErr(err, nil), err
	}

	eng, ok := engine.Get(job.Engine)
	if !ok {
		err := siphonerr.New(siphonerr.Internal,
			"engine %q is not implemented yet (registered: %v)", job.Engine, engine.Names())
		return resultForErr(err, plan), err
	}

	res, err := eng.Convert(ctx, job, plan, sink)
	if err != nil {
		return resultForErr(err, plan), err
	}
	if res == nil {
		res = &core.Result{OK: true, Plan: plan, SourceUntouched: false}
	}
	if res.Plan == nil {
		res.Plan = plan
	}
	return res, nil
}

func resultForErr(err error, plan *core.Plan) *core.Result {
	return &core.Result{
		OK:              false,
		Code:            string(siphonerr.CodeOf(err)),
		Message:         err.Error(),
		Plan:            plan,
		SourceUntouched: siphonerr.SourceUntouched(err),
	}
}
