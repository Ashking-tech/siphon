package siphonerr

import (
	"errors"
	"testing"
)

func TestExitCodes(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{SourceInvalid, 2},
		{NoSpace, 2},
		{Dependency, 5},
		{DependencyOutdated, 5},
		{StitchFailed, 3},
		{VerifyFailed, 3},
		{RecoveryAvailable, 4},
		{Interrupted, 130},
		{Internal, 1},
	}
	for _, c := range cases {
		if got := ExitCode(New(c.code, "x")); got != c.want {
			t.Errorf("%s: got %d, want %d", c.code, got, c.want)
		}
	}
}

func TestExitOverride(t *testing.T) {
	err := New(Dependency, "x").WithExit(2)
	if got := ExitCode(err); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestWrapAndCode(t *testing.T) {
	cause := errors.New("boom")
	err := Wrap(SourceCorrupt, cause, "bad")
	if !IsCode(err, SourceCorrupt) {
		t.Fatal("code not preserved")
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause not preserved")
	}
	if CodeOf(errors.New("plain")) != Internal {
		t.Fatal("plain error must map to internal")
	}
}

func TestUntouched(t *testing.T) {
	err := New(NoSpace, "x").Untouched(true)
	if !SourceUntouched(err) {
		t.Fatal("expected untouched")
	}
}
