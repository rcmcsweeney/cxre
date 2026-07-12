package reset

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestBuildSortsAndCalculatesExpirations(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 500_000_000, time.UTC)
	soon := now.Add(4*time.Hour + 12*time.Minute + 900*time.Millisecond)
	later := now.Add(7*24*time.Hour + 16*time.Hour)

	got := Build(Snapshot{
		AvailableCount:  3,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "never", Status: statusAvailable},
			{ID: "later", Status: statusAvailable, ExpiresAt: &later},
			{ID: "redeemed", Status: statusRedeemed, ExpiresAt: &soon},
			{ID: "soon", Status: statusAvailable, ExpiresAt: &soon},
		},
	}, now)

	if got.AvailableCount != 3 || got.DetailedCount != 3 || got.MissingCount != 0 || !got.Complete {
		t.Fatalf("counts = available:%d detailed:%d missing:%d complete:%t", got.AvailableCount, got.DetailedCount, got.MissingCount, got.Complete)
	}
	if gotIDs := expirationIDs(got.Credits); !reflect.DeepEqual(gotIDs, []string{"soon", "later", "never"}) {
		t.Fatalf("credit order = %v", gotIDs)
	}
	if got.Credits[0].RemainingSeconds != 4*60*60+12*60 || got.Credits[0].Expired {
		t.Fatalf("soon credit = %#v", got.Credits[0])
	}
	if !got.Credits[2].DoesNotExpire || got.Credits[2].ExpiresAt != nil || got.Credits[2].Expired {
		t.Fatalf("non-expiring credit = %#v", got.Credits[2])
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %#v", got.Warnings)
	}
}

func TestBuildPartialDetails(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	tests := []struct {
		name     string
		snapshot Snapshot
		missing  int
		message  string
	}{
		{
			name:     "count only",
			snapshot: Snapshot{AvailableCount: 2},
			missing:  2,
			message:  "Expiration details are unavailable for 2 reset credits.",
		},
		{
			name: "capped list",
			snapshot: Snapshot{
				AvailableCount:  2,
				DetailsProvided: true,
				Credits:         []Credit{{ID: "one", Status: statusAvailable, ExpiresAt: &expires}},
			},
			missing: 1,
			message: "Expiration details are unavailable for 1 reset credit.",
		},
		{
			name: "known unavailable row",
			snapshot: Snapshot{
				AvailableCount:  1,
				DetailsProvided: true,
				Credits:         []Credit{{ID: "busy", Status: statusRedeeming, ExpiresAt: &expires}},
			},
			missing: 1,
			message: "Expiration details are unavailable for 1 reset credit.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Build(test.snapshot, now)
			if got.Complete || got.MissingCount != test.missing || got.DetailedCount != len(got.Credits) {
				t.Fatalf("output = %#v", got)
			}
			if len(got.Warnings) != 1 || got.Warnings[0].Code != WarningCodePartialDetails || got.Warnings[0].Message != test.message {
				t.Fatalf("warnings = %#v", got.Warnings)
			}
		})
	}
}

func TestBuildExplicitZeroIsComplete(t *testing.T) {
	for _, detailsProvided := range []bool{false, true} {
		got := Build(Snapshot{AvailableCount: 0, DetailsProvided: detailsProvided}, time.Now())
		if !got.Complete || got.DetailedCount != 0 || got.MissingCount != 0 {
			t.Fatalf("DetailsProvided=%t: %#v", detailsProvided, got)
		}
		if got.Credits == nil || got.Warnings == nil {
			t.Fatalf("slices must be non-nil: %#v", got)
		}
	}
}

func TestBuildExpiredAndFlooredCountdowns(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 750_000_000, time.UTC)
	past := now.Add(-time.Second)
	exact := now
	underOneSecond := now.Add(999 * time.Millisecond)

	got := Build(Snapshot{
		AvailableCount:  3,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "past", Status: statusAvailable, ExpiresAt: &past},
			{ID: "exact", Status: statusAvailable, ExpiresAt: &exact},
			{ID: "future", Status: statusAvailable, ExpiresAt: &underOneSecond},
		},
	}, now)

	if !got.Credits[0].Expired || got.Credits[0].RemainingSeconds != 0 {
		t.Fatalf("past = %#v", got.Credits[0])
	}
	if !got.Credits[1].Expired || got.Credits[1].RemainingSeconds != 0 {
		t.Fatalf("exact = %#v", got.Credits[1])
	}
	if got.Credits[2].Expired || got.Credits[2].RemainingSeconds != 0 {
		t.Fatalf("sub-second future = %#v", got.Credits[2])
	}
}

func TestBuildUnknownStatusIsIncomplete(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	got := Build(Snapshot{
		AvailableCount:  1,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "known", Status: statusAvailable, ExpiresAt: &expires},
			{ID: "unknown", Status: statusUnknown, ExpiresAt: &expires},
		},
	}, now)

	if got.Complete || got.MissingCount != 0 || got.DetailedCount != 1 {
		t.Fatalf("output = %#v", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != WarningCodeInconsistentDetails {
		t.Fatalf("warnings = %#v", got.Warnings)
	}
}

func TestBuildCountRemainsAuthoritative(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	soon := now.Add(time.Hour)
	later := now.Add(2 * time.Hour)
	got := Build(Snapshot{
		AvailableCount:  1,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "later", Status: statusAvailable, ExpiresAt: &later},
			{ID: "soon", Status: statusAvailable, ExpiresAt: &soon},
		},
	}, now)

	if got.Complete || got.AvailableCount != 1 || got.DetailedCount != 1 || got.MissingCount != 0 {
		t.Fatalf("output = %#v", got)
	}
	if got.Credits[0].ID != "soon" {
		t.Fatalf("retained credit = %q, want soon", got.Credits[0].ID)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != WarningCodeInconsistentDetails {
		t.Fatalf("warnings = %#v", got.Warnings)
	}
}

func TestBuildDoesNotMutateOrAliasInput(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	later := now.Add(2 * time.Hour)
	sooner := now.Add(time.Hour)
	originalSooner := sooner
	snapshot := Snapshot{
		AvailableCount:  2,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "later", Status: statusAvailable, ExpiresAt: &later},
			{ID: "sooner", Status: statusAvailable, ExpiresAt: &sooner},
		},
	}

	got := Build(snapshot, now)
	if snapshot.Credits[0].ID != "later" || snapshot.Credits[1].ID != "sooner" {
		t.Fatalf("input reordered: %#v", snapshot.Credits)
	}
	*snapshot.Credits[1].ExpiresAt = now.Add(99 * time.Hour)
	if !got.Credits[0].ExpiresAt.Equal(originalSooner) {
		t.Fatalf("output aliases input time: %v", got.Credits[0].ExpiresAt)
	}
}

func TestExpirationIDCannotBeSerialized(t *testing.T) {
	encoded, err := json.Marshal(Expiration{ID: "SENTINEL_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("serialized expiration = %s", encoded)
	}
}

func TestEqualExpirationsAndNonExpiringCreditsUseIDTieBreak(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	got := Build(Snapshot{
		AvailableCount:  4,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "z-never", Status: statusAvailable},
			{ID: "z-timed", Status: statusAvailable, ExpiresAt: &expires},
			{ID: "a-never", Status: statusAvailable},
			{ID: "a-timed", Status: statusAvailable, ExpiresAt: &expires},
		},
	}, now)
	if gotIDs := expirationIDs(got.Credits); !reflect.DeepEqual(gotIDs, []string{"a-timed", "z-timed", "a-never", "z-never"}) {
		t.Fatalf("credit order = %v", gotIDs)
	}
}

func TestBuildRepeatedNonEmptyIDRetainsFirstAvailableAndWarns(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	first := now.Add(time.Hour)
	second := now.Add(2 * time.Hour)
	tests := []struct {
		name        string
		credits     []Credit
		wantExpires *time.Time
	}{
		{
			name: "identical available rows",
			credits: []Credit{
				{ID: "same", Status: statusAvailable, ExpiresAt: &first},
				{ID: "same", Status: statusAvailable, ExpiresAt: &first},
			},
			wantExpires: &first,
		},
		{
			name: "conflicting available rows",
			credits: []Credit{
				{ID: "same", Status: statusAvailable, ExpiresAt: &second},
				{ID: "same", Status: statusAvailable, ExpiresAt: &first},
			},
			wantExpires: &second,
		},
		{
			name: "available before redeemed",
			credits: []Credit{
				{ID: "same", Status: statusAvailable, ExpiresAt: &first},
				{ID: "same", Status: statusRedeemed, ExpiresAt: &second},
			},
			wantExpires: &first,
		},
		{
			name: "available after redeemed",
			credits: []Credit{
				{ID: "same", Status: statusRedeemed, ExpiresAt: &first},
				{ID: "same", Status: statusAvailable, ExpiresAt: &second},
			},
			wantExpires: &second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Build(Snapshot{
				AvailableCount:  1,
				DetailsProvided: true,
				Credits:         test.credits,
			}, now)

			if got.Complete || got.DetailedCount != 1 || got.MissingCount != 0 || len(got.Credits) != 1 {
				t.Fatalf("output = %#v", got)
			}
			if got.Credits[0].ID != "same" || !got.Credits[0].ExpiresAt.Equal(*test.wantExpires) {
				t.Fatalf("retained credit = %#v, want expiry %v", got.Credits[0], test.wantExpires)
			}
			assertWarningCodes(t, got.Warnings, WarningCodeInconsistentDetails)
		})
	}
}

func TestBuildRepeatedNonEmptyIDCanLeaveAuthoritativeDetailMissing(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	got := Build(Snapshot{
		AvailableCount:  2,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "same", Status: statusAvailable, ExpiresAt: &expires},
			{ID: "same", Status: statusAvailable, ExpiresAt: &expires},
		},
	}, now)

	if got.Complete || got.DetailedCount != 1 || got.MissingCount != 1 || len(got.Credits) != 1 {
		t.Fatalf("output = %#v", got)
	}
	assertWarningCodes(t, got.Warnings, WarningCodePartialDetails, WarningCodeInconsistentDetails)
}

func TestBuildRepeatedNonAvailableIDIsInconsistent(t *testing.T) {
	got := Build(Snapshot{
		AvailableCount:  0,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "same", Status: statusRedeeming},
			{ID: "same", Status: statusRedeemed},
		},
	}, time.Now())

	if got.Complete || got.DetailedCount != 0 || got.MissingCount != 0 || len(got.Credits) != 0 {
		t.Fatalf("output = %#v", got)
	}
	assertWarningCodes(t, got.Warnings, WarningCodeInconsistentDetails)
}

func TestBuildEmptyAvailableIDsRemainDistinctAndInconsistent(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	soon := now.Add(time.Hour)
	later := now.Add(2 * time.Hour)
	got := Build(Snapshot{
		AvailableCount:  3,
		DetailsProvided: true,
		Credits: []Credit{
			{Status: statusAvailable, ExpiresAt: &later},
			{Status: statusAvailable},
			{Status: statusAvailable, ExpiresAt: &soon},
		},
	}, now)

	if got.Complete || got.DetailedCount != 3 || got.MissingCount != 0 || len(got.Credits) != 3 {
		t.Fatalf("output = %#v", got)
	}
	if !got.Credits[0].ExpiresAt.Equal(soon) || !got.Credits[1].ExpiresAt.Equal(later) || !got.Credits[2].DoesNotExpire {
		t.Fatalf("credits = %#v", got.Credits)
	}
	assertWarningCodes(t, got.Warnings, WarningCodeInconsistentDetails)
}

func TestBuildEmptyKnownUnavailableIDIsInconsistent(t *testing.T) {
	got := Build(Snapshot{
		AvailableCount:  0,
		DetailsProvided: true,
		Credits:         []Credit{{Status: statusRedeemed}},
	}, time.Now())

	if got.Complete || got.DetailedCount != 0 || got.MissingCount != 0 {
		t.Fatalf("output = %#v", got)
	}
	assertWarningCodes(t, got.Warnings, WarningCodeInconsistentDetails)
}

func TestBuildRepeatedIDWithUnknownThenAvailableRetainsUsefulRow(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	got := Build(Snapshot{
		AvailableCount:  1,
		DetailsProvided: true,
		Credits: []Credit{
			{ID: "same", Status: statusUnknown},
			{ID: "same", Status: statusAvailable, ExpiresAt: &expires},
		},
	}, now)

	if got.Complete || got.DetailedCount != 1 || got.MissingCount != 0 || len(got.Credits) != 1 {
		t.Fatalf("output = %#v", got)
	}
	if got.Credits[0].ID != "same" || !got.Credits[0].ExpiresAt.Equal(expires) {
		t.Fatalf("retained credit = %#v", got.Credits[0])
	}
	assertWarningCodes(t, got.Warnings, WarningCodeInconsistentDetails)
}

func assertWarningCodes(t *testing.T, warnings []Warning, want ...string) {
	t.Helper()
	got := make([]string, len(warnings))
	for i, warning := range warnings {
		got[i] = warning.Code
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warning codes = %v, want %v; warnings = %#v", got, want, warnings)
	}
}

func expirationIDs(credits []Expiration) []string {
	ids := make([]string, len(credits))
	for i, credit := range credits {
		ids[i] = credit.ID
	}
	return ids
}
