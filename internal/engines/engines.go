// Package engines is the composition root. It wires concrete engine
// implementations into the registry so the orchestrator and the CLI see them;
// engine implementations themselves never import this package.
package engines

import (
	"siphon/internal/engine"
	"siphon/internal/engine/atomiccopy"
	"siphon/internal/formats"
)

func init() {
	engine.Register(formats.EngineAtomicCopy, atomiccopy.New())
}
