// Package progress keeps engines UI-agnostic. The CLI, kdialog wrapper, and a
// future GUI consume the same events.
package progress

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Event is one progress observation.
type Event struct {
	Phase   string  `json:"phase"`
	Current int64   `json:"current,omitempty"`
	Total   int64   `json:"total,omitempty"`
	Percent float64 `json:"percent,omitempty"`
	Message string  `json:"message,omitempty"`
}

// Sink receives progress and warnings. Implementations must be safe for use
// from one goroutine; engines do not emit concurrently.
type Sink interface {
	Emit(Event)
	Warn(string)
	Close() error
}

// Noop discards everything.
type Noop struct{}

func (Noop) Emit(Event)   {}
func (Noop) Warn(string)  {}
func (Noop) Close() error { return nil }

// Terminal renders a single-line progress indicator to w (normally stderr).
type Terminal struct {
	w       io.Writer
	lastPct int
	started bool
}

func NewTerminal(w io.Writer) *Terminal { return &Terminal{w: w, lastPct: -1} }

func (t *Terminal) Emit(e Event) {
	pct := int(e.Percent)
	if e.Total > 0 {
		pct = int(float64(e.Current) * 100 / float64(e.Total))
	}
	if pct == t.lastPct && e.Message == "" {
		return
	}
	t.lastPct = pct
	t.started = true
	if e.Message != "" {
		fmt.Fprintf(t.w, "\r[%s] %3d%% %s\033[K", e.Phase, pct, e.Message)
		return
	}
	fmt.Fprintf(t.w, "\r[%s] %3d%%\033[K", e.Phase, pct)
}

func (t *Terminal) Warn(msg string) {
	fmt.Fprintf(t.w, "\r\033[Kwarning: %s\n", msg)
}

func (t *Terminal) Close() error {
	if t.started {
		fmt.Fprintln(t.w)
	}
	return nil
}

// JSON emits one JSON object per line. This is the GUI-facing stream.
type JSON struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func NewJSON(w io.Writer) *JSON {
	return &JSON{enc: json.NewEncoder(w)}
}

func (j *JSON) Emit(e Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	_ = j.enc.Encode(e)
}

func (j *JSON) Warn(msg string) {
	j.Emit(Event{Phase: "warning", Message: msg})
}

func (j *JSON) Close() error { return nil }
