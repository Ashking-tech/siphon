package pdf

import (
	"strings"
	"testing"

	"siphon/internal/siphonerr"
)

func TestVerifyHeaderErrorPreservesPDFVersion(t *testing.T) {
	err := Verify([]byte("not a PDF"), Expect{})
	if err == nil {
		t.Fatal("Verify accepted data without a PDF header")
	}
	if !siphonerr.IsCode(err, siphonerr.VerifyFailed) {
		t.Fatalf("error code = %s, want %s", siphonerr.CodeOf(err), siphonerr.VerifyFailed)
	}
	msg := siphonerr.MessageOf(err)
	if !strings.Contains(msg, "header is not %PDF-1.7") {
		t.Fatalf("message = %q, want it to contain %q", msg, "header is not %PDF-1.7")
	}
	if strings.Contains(msg, "%!") {
		t.Fatalf("message contains a formatting artifact: %q", msg)
	}
}
