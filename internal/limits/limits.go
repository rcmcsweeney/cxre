// Package limits normalizes Codex usage-limit windows into presentation-ready
// data. It is transport-neutral so changes to the app-server protocol stay out
// of terminal and JSON rendering.
package limits

import "time"

// WindowSnapshot is one usage window returned by Codex. A nil ResetsAt means
// the percentage is available but the server did not provide a reset time.
type WindowSnapshot struct {
	UsedPercent float64
	ResetsAt    *time.Time
}

// Snapshot contains the usage windows CXRE currently supports. A nil window
// means that it was unavailable in the server response.
type Snapshot struct {
	FiveHour *WindowSnapshot
	Weekly   *WindowSnapshot
}

// Window is a normalized usage window. ResetsAt stays nil when Codex reports a
// percentage without reset timing. When it is present, RemainingSeconds is
// floored to whole seconds. A future reset less than one second away therefore
// has zero seconds remaining while ResetDue remains false.
type Window struct {
	UsedPercent      float64
	ResetsAt         *time.Time
	RemainingSeconds int64
	ResetDue         bool
}

// Output is the normalized usage-limit view consumed by presentation code.
// Nil windows remain explicitly unavailable.
type Output struct {
	FiveHour *Window
	Weekly   *Window
}

// Build normalizes snapshot at now without mutating or retaining pointers into
// the input.
func Build(snapshot Snapshot, now time.Time) Output {
	return Output{
		FiveHour: buildWindow(snapshot.FiveHour, now),
		Weekly:   buildWindow(snapshot.Weekly, now),
	}
}

func buildWindow(snapshot *WindowSnapshot, now time.Time) *Window {
	if snapshot == nil {
		return nil
	}

	window := &Window{UsedPercent: snapshot.UsedPercent}
	if snapshot.ResetsAt == nil {
		return window
	}

	resetsAt := *snapshot.ResetsAt
	window.ResetsAt = &resetsAt
	if !resetsAt.After(now) {
		window.ResetDue = true
		return window
	}

	window.RemainingSeconds = int64(resetsAt.Sub(now) / time.Second)
	return window
}
