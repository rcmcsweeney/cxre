//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package cli

import (
	"syscall"
	"testing"
)

func TestTerminationIsSilentAndRestoresDefaultBeforeCancellation(t *testing.T) {
	testSignalExit(t, syscall.SIGTERM, 143)
}

func TestHandledSignalsIncludeInterruptAndTermination(t *testing.T) {
	signals := handledSignals()
	if len(signals) != 2 || signals[0] != syscall.SIGINT || signals[1] != syscall.SIGTERM {
		t.Fatalf("handledSignals() = %v, want SIGINT and SIGTERM", signals)
	}
}
