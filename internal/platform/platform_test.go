package platform

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"siphon/internal/formats"
)

func writeFile(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateTemp(t *testing.T) {
	dir := t.TempDir()
	f, err := CreateTemp(dir, "film.mkv", ".tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if filepath.Dir(f.Name()) != dir {
		t.Fatalf("temp not in source dir: %s", f.Name())
	}
	base := filepath.Base(f.Name())
	if base[0] != '.' {
		t.Fatalf("temp not hidden: %s", base)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRenameNoReplace(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a", []byte("a"))
	b := writeFile(t, dir, "b", []byte("b"))

	if err := RenameNoReplace(a, b); err == nil {
		t.Fatal("replaced an existing target")
	}
	c := filepath.Join(dir, "c")
	if err := RenameNoReplace(a, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMagic(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		f    formats.Format
		b    []byte
	}{
		{"a.wav", formats.WAV, []byte("RIFF\x00\x00\x00\x00WAVEfmt ")},
		{"a.mkv", formats.MKV, []byte{0x1a, 0x45, 0xdf, 0xa3, 0x00}},
		{"a.png", formats.PNG, []byte("\x89PNG\r\n\x1a\n")},
		{"a.jpg", formats.JPG, []byte{0xff, 0xd8, 0xff, 0xe0}},
		{"a.webp", formats.WEBP, []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")},
	}
	for _, c := range cases {
		path := writeFile(t, dir, c.name, c.b)
		if err := ValidateMagic(path, c.f); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	bad := writeFile(t, dir, "bad.png", []byte("not a png"))
	if err := ValidateMagic(bad, formats.PNG); err == nil {
		t.Fatal("mismatched magic accepted")
	}
}

func TestSparseProbeAndFreeSpace(t *testing.T) {
	dir := t.TempDir()
	if err := SparseProbe(dir, 64<<20); err != nil {
		t.Fatalf("sparse probe: %v", err)
	}
	free, err := FreeSpace(dir)
	if err != nil {
		t.Fatal(err)
	}
	if free <= 0 {
		t.Fatalf("free = %d", free)
	}
}

func TestCopyMetadata(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "src", []byte("src"))
	dst := writeFile(t, dir, "dst", []byte("dst"))

	if err := unix.Setxattr(src, "user.siphon-test", []byte("value"), 0); err != nil {
		t.Skipf("xattrs unsupported: %v", err)
	}
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}

	warnings, err := CopyMetadata(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	_ = warnings

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
	size, err := unix.Getxattr(dst, "user.siphon-test", nil)
	if err != nil || size == 0 {
		t.Fatalf("xattr not copied: %d %v", size, err)
	}
}
