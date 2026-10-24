package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSaveLoad(t *testing.T) {
	dir := t.TempDir()
	store, j, err := Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	if j.Seq != 1 {
		t.Fatalf("initial seq = %d, want 1", j.Seq)
	}
	j.Phase = "encoding"
	j.Segments = []Segment{{I: 0, SrcOff: 0, SrcEnd: 10, State: "done"}}
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store2, got, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	if got.Seq != 2 || got.Phase != "encoding" || len(got.Segments) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.CRC32C == "" {
		t.Fatal("CRC missing")
	}
}

func TestTamperDetected(t *testing.T) {
	dir := t.TempDir()
	store, j, err := Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	j.Phase = "encoding"
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	store.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(b), `"phase": "encoding"`, `"phase": "verifying"`, 1)
	if tampered == string(b) {
		t.Fatal("test did not change the journal")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path); err == nil {
		t.Fatal("tampered journal was accepted")
	}
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	store, _, err := Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Remove()

	paths, err := Find(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("found %d journals", len(paths))
	}
	if filepath.Dir(paths[0]) != dir {
		t.Fatalf("wrong dir: %s", paths[0])
	}
	if _, err := FindOne(dir, "other"); err == nil {
		t.Fatal("expected no journal error")
	}
}

func TestVerifySource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.wav")
	if err := os.WriteFile(src, []byte("RIFF....WAVE"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewSource(src)
	if err != nil {
		t.Fatal(err)
	}
	j := &Journal{Job: Job{Mode: "safe", Source: s}}
	if err := VerifySource(j, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifySource(j, src); err == nil {
		t.Fatal("changed source accepted")
	}
}
