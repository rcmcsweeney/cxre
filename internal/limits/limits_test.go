package limits

import (
	"testing"
	"time"
)

func TestBuildNormalizesIndependentWindows(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 500_000_000, time.UTC)
	fiveHourReset := now.Add(3*time.Hour + 12*time.Minute + 900*time.Millisecond)
	weeklyReset := now.Add(4*24*time.Hour + 7*time.Hour)

	got := Build(Snapshot{
		FiveHour: &WindowSnapshot{UsedPercent: 37, ResetsAt: &fiveHourReset},
		Weekly:   &WindowSnapshot{UsedPercent: 68.5, ResetsAt: &weeklyReset},
	}, now)

	if got.FiveHour == nil || got.FiveHour.UsedPercent != 37 || got.FiveHour.RemainingSeconds != 3*60*60+12*60 || got.FiveHour.ResetDue {
		t.Fatalf("five-hour window = %#v", got.FiveHour)
	}
	if got.Weekly == nil || got.Weekly.UsedPercent != 68.5 || got.Weekly.RemainingSeconds != 4*24*60*60+7*60*60 || got.Weekly.ResetDue {
		t.Fatalf("weekly window = %#v", got.Weekly)
	}
}

func TestBuildKeepsPercentageWhenResetTimeIsUnavailable(t *testing.T) {
	got := Build(Snapshot{
		FiveHour: &WindowSnapshot{UsedPercent: 42},
	}, time.Now())

	if got.FiveHour == nil || got.FiveHour.UsedPercent != 42 || got.FiveHour.ResetsAt != nil || got.FiveHour.RemainingSeconds != 0 || got.FiveHour.ResetDue {
		t.Fatalf("five-hour window = %#v", got.FiveHour)
	}
	if got.Weekly != nil {
		t.Fatalf("output = %#v", got)
	}
}

func TestBuildResetDueAndFlooredRemaining(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 750_000_000, time.UTC)
	past := now.Add(-time.Second)
	exact := now
	subsecondFuture := now.Add(999 * time.Millisecond)

	tests := []struct {
		name      string
		reset     time.Time
		resetDue  bool
		remaining int64
	}{
		{name: "past", reset: past, resetDue: true},
		{name: "exact", reset: exact, resetDue: true},
		{name: "sub-second future", reset: subsecondFuture},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Build(Snapshot{FiveHour: &WindowSnapshot{ResetsAt: &test.reset}}, now)
			if got.FiveHour == nil || got.FiveHour.ResetDue != test.resetDue || got.FiveHour.RemainingSeconds != test.remaining {
				t.Fatalf("window = %#v", got.FiveHour)
			}
		})
	}
}

func TestBuildDoesNotAliasInput(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	want := reset
	snapshot := Snapshot{FiveHour: &WindowSnapshot{UsedPercent: 12, ResetsAt: &reset}}

	got := Build(snapshot, now)
	*snapshot.FiveHour.ResetsAt = now.Add(99 * time.Hour)

	if got.FiveHour == nil || got.FiveHour.ResetsAt == nil || !got.FiveHour.ResetsAt.Equal(want) {
		t.Fatalf("output aliases input: %#v", got.FiveHour)
	}
}
