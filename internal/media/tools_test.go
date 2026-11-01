package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestToolVersionFlags(t *testing.T) {
	cases := []struct {
		name   string
		arg    string
		output string
		want   string
	}{
		{"ffmpeg", "-version", "ffmpeg version 8.1.3", "8.1.3"},
		{"lame", "--version", "LAME version 3.100", "3.100"},
		{"pdfinfo", "-v", "pdfinfo version 26.01.0", "26.01.0"},
		{"pdftoppm", "-v", "pdftoppm version 26.01.0", "26.01.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			script := "#!/bin/sh\n" +
				"if [ \"$1\" != \"" + tc.arg + "\" ]; then\n" +
				"  echo \"unexpected argument: $1\" >&2\n" +
				"  exit 1\n" +
				"fi\n" +
				"printf '%s\\n' '" + tc.output + "'\n"
			if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := toolVersion(context.Background(), tc.name, path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}
