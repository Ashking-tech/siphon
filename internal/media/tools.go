// Package media wraps the external tools. Siphon never links an encoder.
package media

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"siphon/internal/formats"
	"siphon/internal/siphonerr"
)

// Tool is a discovered external program.
type Tool struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

var minimums = map[string]string{
	"ffmpeg":  "5.0",
	"ffprobe": "5.0",
	"lame":    "3.100",
	"cwebp":   "1.0",
	"dwebp":   "1.0",
}

// RequiredFor returns the external tools a pair needs.
func RequiredFor(source, target formats.Format) []string {
	tools := []string{"ffmpeg", "ffprobe"}
	if source == formats.WAV || target == formats.MP3 {
		tools = append(tools, "lame")
	}
	if target == formats.WEBP {
		tools = append(tools, "cwebp")
	}
	if source == formats.WEBP {
		tools = append(tools, "dwebp")
	}
	return tools
}

// Check discovers each tool and enforces its minimum version. During
// pre-flight, callers convert failures into refusals (exit 2).
func Check(ctx context.Context, names []string) (map[string]string, error) {
	versions := make(map[string]string, len(names))
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, siphonerr.New(siphonerr.Dependency, "%s is not installed or not in PATH", name)
		}
		version, err := toolVersion(ctx, name, path)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.Dependency, err, "cannot determine %s version", name)
		}
		if min := minimums[name]; min != "" && numericVersion(version) != "" && !versionAtLeast(version, min) {
			return nil, siphonerr.New(siphonerr.DependencyOutdated,
				"%s %s is too old; %s or newer is required", name, version, min)
		}
		versions[name] = version
	}
	return versions, nil
}

// Inspect discovers tools without enforcing minimums (doctor).
func Inspect(ctx context.Context, names []string) []Tool {
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		t := Tool{Name: name}
		path, err := exec.LookPath(name)
		if err != nil {
			out = append(out, t)
			continue
		}
		t.Path = path
		t.Version, _ = toolVersion(ctx, name, path)
		out = append(out, t)
	}
	return out
}

// AllTools is the doctor set.
func AllTools() []string { return []string{"ffmpeg", "ffprobe", "lame", "cwebp", "dwebp"} }

func toolVersion(ctx context.Context, name, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"-version"}
	switch name {
	case "lame":
		args = []string{"--version"}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		// Some tools print the version and exit non-zero.
		if buf.Len() == 0 {
			return "", err
		}
	}
	return parseVersion(name, buf.String()), nil
}

var versionRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)+)`)

func parseVersion(name, output string) string {
	lower := strings.ToLower(output)
	var line string
	for _, l := range strings.Split(lower, "\n") {
		if strings.Contains(l, name) || strings.Contains(l, "version") {
			line = l
			break
		}
	}
	if line == "" {
		line = lower
	}
	if m := versionRE.FindString(line); m != "" {
		return m
	}
	return strings.TrimSpace(line)
}

func numericVersion(v string) string { return versionRE.FindString(v) }

func versionAtLeast(got, min string) bool {
	g, ok := parseParts(numericVersion(got))
	if !ok {
		return true // unparseable git build: permit, doctor shows the raw string
	}
	m, ok := parseParts(min)
	if !ok {
		return true
	}
	for i := 0; i < len(g) || i < len(m); i++ {
		var a, b int
		if i < len(g) {
			a = g[i]
		}
		if i < len(m) {
			b = m[i]
		}
		if a != b {
			return a > b
		}
	}
	return true
}

func parseParts(v string) ([]int, bool) {
	if v == "" {
		return nil, false
	}
	fields := strings.Split(v, ".")
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// VersionString is a helper for messages.
func VersionString(tools map[string]string) string {
	var parts []string
	for _, name := range []string{"ffmpeg", "ffprobe", "lame", "cwebp", "dwebp"} {
		if v, ok := tools[name]; ok {
			parts = append(parts, fmt.Sprintf("%s %s", name, v))
		}
	}
	return strings.Join(parts, ", ")
}
