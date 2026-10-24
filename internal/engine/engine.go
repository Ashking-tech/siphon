// Package engine defines the conversion interface and a registry. Engines are
// UI-agnostic and receive a validated plan.
package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/progress"
)

// Engine converts one planned job.
type Engine interface {
	Name() string
	Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error)
}

var (
	mu       sync.RWMutex
	registry = make(map[formats.EngineKind]Engine)
)

// Register wires an engine. It panics on duplicate registration.
func Register(kind formats.EngineKind, e Engine) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[kind]; exists {
		panic(fmt.Sprintf("engine %s registered twice", kind))
	}
	registry[kind] = e
}

// Get returns the engine for kind.
func Get(kind formats.EngineKind) (Engine, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := registry[kind]
	return e, ok
}

// Names lists registered engines (diagnostics).
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}
