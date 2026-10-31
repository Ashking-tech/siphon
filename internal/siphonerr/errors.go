// Package siphonerr defines the structured error contract shared by the CLI,
// the engines, and the GUI-facing JSON output.
package siphonerr

import (
	"errors"
	"fmt"
)

// Code is a stable machine-readable error identifier. The GUI and the KDE
// wrapper switch on these; they never parse prose.
type Code string

const (
	Usage    Code = "E_USAGE"
	Internal Code = "E_INTERNAL"

	// Pre-flight refusals (exit 2, source byte-identical).
	SourceInvalid      Code = "E_SOURCE_INVALID"
	FormatUnsupported  Code = "E_FORMAT_UNSUPPORTED"
	ModeUnsupported    Code = "E_MODE_UNSUPPORTED"
	Multilink          Code = "E_MULTILINK"
	Permission         Code = "E_PERMISSION"
	TargetExists       Code = "E_TARGET_EXISTS"
	CrossDevice        Code = "E_CROSS_DEVICE"
	CodecIncompatible  Code = "E_CODEC_INCOMPATIBLE"
	MetadataDrop       Code = "E_METADATA_DROP"
	NoSpace            Code = "E_NO_SPACE"
	InplaceInfeasible  Code = "E_INPLACE_INFEASIBLE"
	FSNoSparse         Code = "E_FS_NO_SPARSE"
	StaleJournal       Code = "E_STALE_JOURNAL"
	SourceCorrupt      Code = "E_SOURCE_CORRUPT"
	Dependency         Code = "E_DEPENDENCY"
	DependencyOutdated Code = "E_DEPENDENCY_OUTDATED"
	RootDenied         Code = "E_ROOT_DENIED"
	ConfirmationNeeded Code = "E_CONFIRMATION_REQUIRED"
	AlphaUnsupported   Code = "E_ALPHA_UNSUPPORTED"
	SourceChanged      Code = "E_SOURCE_CHANGED"

	// v0.2 document/image and media refusals (spec §6.7). Additive only.
	PDFMultipage      Code = "E_PDF_MULTIPAGE"
	PDFEncrypted      Code = "E_PDF_ENCRYPTED"
	PDFInvalid        Code = "E_PDF_INVALID"
	PDFTooLarge       Code = "E_PDF_TOO_LARGE"
	TranscodeRequired Code = "E_TRANSCODE_REQUIRED"
	NoAudio           Code = "E_NO_AUDIO"
	AudioTrack        Code = "E_AUDIO_TRACK"

	// Verification failures (exit 3).
	StitchFailed Code = "E_STITCH_FAILED"
	SegmentPlan  Code = "E_SEGMENT_PLAN"
	VerifyFailed Code = "E_VERIFY_FAILED"

	// Runtime conditions.
	NoSpaceRuntime    Code = "E_NO_SPACE_RUNTIME"
	RecoveryAvailable Code = "E_RECOVERY_AVAILABLE"
	Interrupted       Code = "E_INTERRUPTED"
)

// Error is Siphon's structured error. Exit overrides the code's default exit
// status when nonzero (for example, a dependency missing during pre-flight is
// still a refusal, exit 2, not the runtime dependency status 5).
type Error struct {
	Code            Code
	Message         string
	Cause           error
	Exit            int
	SourceUntouched bool
	Hints           map[string]bool
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// MessageOf returns the human-readable error text without the stable code
// prefix. JSON consumers receive the code separately.
func MessageOf(err error) string {
	if err == nil {
		return ""
	}
	var se *Error
	if errors.As(err, &se) {
		if se.Cause != nil {
			return fmt.Sprintf("%s: %v", se.Message, se.Cause)
		}
		return se.Message
	}
	return err.Error()
}

// New builds an error with a formatted message.
func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap builds an error that retains an underlying cause.
func Wrap(code Code, cause error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// WithExit overrides the exit status for this error.
func (e *Error) WithExit(exit int) *Error {
	e.Exit = exit
	return e
}

// Untouched records whether the source is guaranteed byte-identical.
func (e *Error) Untouched(v bool) *Error {
	e.SourceUntouched = v
	return e
}

// WithHints attaches boolean retry hints for the JSON/KDE surfaces.
func (e *Error) WithHints(hints map[string]bool) *Error {
	if len(hints) > 0 {
		e.Hints = make(map[string]bool, len(hints))
		for k, v := range hints {
			e.Hints[k] = v
		}
	}
	return e
}

// HintsOf returns the structured retry hints attached to err, if any.
func HintsOf(err error) map[string]bool {
	var se *Error
	if errors.As(err, &se) {
		return se.Hints
	}
	return nil
}

// CodeOf returns the structured code of err, or E_INTERNAL when it is not a
// Siphon error.
func CodeOf(err error) Code {
	var se *Error
	if errors.As(err, &se) {
		return se.Code
	}
	return Internal
}

// IsCode reports whether err carries code.
func IsCode(err error, code Code) bool {
	return CodeOf(err) == code
}

// ExitCode maps an error to the process exit status defined in spec §5.2.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var se *Error
	if errors.As(err, &se) {
		if se.Exit != 0 {
			return se.Exit
		}
		return defaultExit(se.Code)
	}
	return 1
}

// SourceUntouched reports whether the error guarantees an untouched source.
func SourceUntouched(err error) bool {
	var se *Error
	if errors.As(err, &se) {
		return se.SourceUntouched
	}
	return false
}

func defaultExit(code Code) int {
	switch code {
	case Interrupted:
		return 130
	case StitchFailed, SegmentPlan, VerifyFailed:
		return 3
	case RecoveryAvailable:
		return 4
	case Dependency, DependencyOutdated:
		return 5
	}
	if isRefusal(code) {
		return 2
	}
	return 1
}

func isRefusal(code Code) bool {
	switch code {
	case Usage, SourceInvalid, FormatUnsupported, ModeUnsupported, Multilink,
		Permission, TargetExists, CrossDevice, CodecIncompatible, MetadataDrop,
		NoSpace, InplaceInfeasible, FSNoSparse, StaleJournal, SourceCorrupt,
		Dependency, DependencyOutdated, RootDenied, ConfirmationNeeded,
		AlphaUnsupported, SourceChanged, PDFMultipage, PDFEncrypted, PDFInvalid,
		PDFTooLarge, TranscodeRequired, NoAudio, AudioTrack:
		return true
	}
	return false
}
