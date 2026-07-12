//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package cli

import (
	"os"
	"syscall"
)

func handledSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func exitCodeForSignal(received os.Signal) int {
	if received == syscall.SIGTERM {
		return 128 + int(syscall.SIGTERM)
	}
	return 128 + int(syscall.SIGINT)
}
