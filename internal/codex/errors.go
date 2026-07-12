// Package codex talks to the locally installed Codex app server.
//
// The package deliberately exposes only sanitized, allowlisted errors. Raw
// protocol responses and child-process output can contain account data and
// must never be shown to users or logs.
package codex

import "errors"

// Code is a stable, machine-readable failure classification.
type Code string

const (
	CodeCodexNotFound   Code = "codex_not_found"
	CodeAuthMissing     Code = "auth_missing"
	CodeUnsupportedAuth Code = "unsupported_auth"
	CodeCodexTooOld     Code = "codex_too_old"
	CodeTimeout         Code = "timeout"
	CodeNetwork         Code = "network"
	CodeProtocol        Code = "protocol"
)

// Error is safe to present to a user. It intentionally retains no raw
// subprocess or protocol text.
type Error struct {
	Code    Code
	Message string
	Action  string
}

func (e *Error) Error() string { return e.Message }

// CodeOf returns the stable code carried by err. Unexpected errors are treated
// as protocol failures so callers never need to print an untrusted error.
func CodeOf(err error) Code {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return CodeProtocol
}

func failure(code Code, _ error) *Error {
	message, action := errorText(code)
	return &Error{Code: code, Message: message, Action: action}
}

func errorText(code Code) (message, action string) {
	switch code {
	case CodeCodexNotFound:
		return "Unable to find the Codex CLI.", "Install Codex 0.143.0 or newer, then run cxre again."
	case CodeAuthMissing:
		return "Unable to find Codex authentication.", "Run `codex login`, sign in with ChatGPT, then run `cxre` again."
	case CodeUnsupportedAuth:
		return "CXRE requires a ChatGPT Codex sign-in.", "Run `codex logout`, then `codex login` and sign in with ChatGPT."
	case CodeCodexTooOld:
		return "This Codex CLI does not support reset credit expirations.", "Update Codex to version 0.143.0 or newer, then try again."
	case CodeTimeout:
		return "Codex did not respond in time.", "Try again. If the problem continues, check your network connection."
	case CodeNetwork:
		return "Codex could not reach the service.", "Check your network connection, then try again."
	default:
		return "Codex returned an unexpected response.", "Update Codex and try again."
	}
}
