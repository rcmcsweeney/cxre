package presentation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/rcmcsweeney/cxre/internal/limits"
	"github.com/rcmcsweeney/cxre/internal/reset"
)

func TestRenderHumanTable(t *testing.T) {
	zone := time.FixedZone("NZST", 12*60*60)
	expires := time.Date(2026, time.July, 12, 20, 42, 17, 0, zone)
	fiveHourReset := time.Date(2026, time.July, 12, 18, 0, 0, 0, zone)
	weeklyReset := time.Date(2026, time.July, 18, 12, 0, 0, 0, zone)
	result := reset.Output{
		AvailableCount: 1,
		DetailedCount:  1,
		Complete:       true,
		Credits: []reset.Expiration{{
			ExpiresAt:        &expires,
			RemainingSeconds: 4*60*60 + 12*60,
		}},
	}
	report := Report{
		Limits: limits.Output{
			FiveHour: &limits.Window{UsedPercent: 37, ResetsAt: &fiveHourReset, RemainingSeconds: 2*60*60 + 30*60},
			Weekly:   &limits.Window{UsedPercent: 68.5, ResetsAt: &weeklyReset, RemainingSeconds: 5*24*60*60 + 20*60*60},
		},
		Resets: result,
	}

	var output, warnings strings.Builder
	err := RenderHuman(&output, &warnings, report, Options{Location: zone, Width: 80})
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"CXRE — Codex Reset Expirations",
		"Usage limits",
		"Window",
		"Left",
		"5h",
		"63%",
		"Sun 12 Jul 2026 6:00:00 PM NZST",
		"2h 30m",
		"Weekly",
		"31.5%",
		"5d 20h",
		"Available reset credits: 1",
		"Expires",
		"Sun 12 Jul 2026 8:42:17 PM NZST",
		"4h 12m",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("human output missing %q:\n%s", expected, output.String())
		}
	}
	if warnings.Len() != 0 {
		t.Fatalf("unexpected warnings: %s", warnings.String())
	}
}

func TestRenderHumanNarrowAndWarning(t *testing.T) {
	expires := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	fiveHourReset := expires.Add(time.Hour)
	result := reset.Output{
		AvailableCount: 2,
		DetailedCount:  1,
		MissingCount:   1,
		Credits: []reset.Expiration{{
			ExpiresAt:        &expires,
			RemainingSeconds: 30,
		}},
		Warnings: []reset.Warning{{
			Code:    "partial_reset_credit_details",
			Message: "Expiration details are unavailable for 1 reset credit.",
		}},
	}
	report := Report{
		Limits: limits.Output{FiveHour: &limits.Window{UsedPercent: 95, ResetsAt: &fiveHourReset, RemainingSeconds: 60 * 60}},
		Resets: result,
	}

	var output, warnings strings.Builder
	if err := RenderHuman(&output, &warnings, report, Options{Location: time.UTC, Width: 40, Unicode: true}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"5h\nLeft:      5%\nResets:    Sun 12 Jul 2026 9:00:00 AM UTC\nRemaining: 1h",
		"Weekly\nLeft:      —\nResets:    —\nRemaining: —",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing narrow limit %q:\n%s", expected, output.String())
		}
	}
	if !strings.Contains(output.String(), "Expires:   Sun 12 Jul 2026 8:00:00 AM UTC") {
		t.Fatalf("missing stacked output:\n%s", output.String())
	}
	if !strings.Contains(warnings.String(), "! Expiration details are unavailable") {
		t.Fatalf("missing warning: %s", warnings.String())
	}
}

func TestRenderHumanKeepsLeftPercentWithoutResetTime(t *testing.T) {
	report := Report{Limits: limits.Output{
		FiveHour: &limits.Window{UsedPercent: 37},
	}}

	var output, warnings strings.Builder
	if err := RenderHuman(&output, &warnings, report, Options{Location: time.UTC, Width: 40}); err != nil {
		t.Fatal(err)
	}
	if expected := "5h\nLeft:      63%\nResets:    —\nRemaining: —"; !strings.Contains(output.String(), expected) {
		t.Fatalf("missing percentage-only limit %q:\n%s", expected, output.String())
	}
}

func TestRenderHumanZeroStillShowsInconsistencyWarning(t *testing.T) {
	result := reset.Output{
		AvailableCount: 0,
		Complete:       false,
		Warnings: []reset.Warning{{
			Code:    "inconsistent_reset_credit_details",
			Message: "Some reset-credit details could not be classified reliably.",
		}},
	}

	var output, warnings strings.Builder
	if err := RenderHuman(&output, &warnings, Report{Resets: result}, Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No reset credits are currently available.") ||
		!strings.Contains(warnings.String(), "Warning: Some reset-credit details") {
		t.Fatalf("output=%q warnings=%q", output.String(), warnings.String())
	}
}

func TestRenderJSONStableAndSanitized(t *testing.T) {
	now := time.Date(2026, time.July, 12, 13, 14, 49, 999, time.UTC)
	expires := now.Add(90 * time.Minute).Truncate(time.Second)
	fiveHourReset := now.Add(4 * time.Hour).Truncate(time.Second)
	weeklyReset := now.Add(7 * 24 * time.Hour).Truncate(time.Second)
	result := reset.Output{
		AvailableCount: 2,
		DetailedCount:  2,
		Complete:       true,
		Credits: []reset.Expiration{
			{ID: "secret-credit-id", ExpiresAt: &expires, RemainingSeconds: 5400},
			{ID: "another-secret", DoesNotExpire: true},
		},
		Warnings: []reset.Warning{},
	}
	report := Report{
		Limits: limits.Output{
			FiveHour: &limits.Window{UsedPercent: 42, ResetsAt: &fiveHourReset, RemainingSeconds: 4 * 60 * 60},
			Weekly:   &limits.Window{UsedPercent: 68.5, ResetsAt: &weeklyReset, RemainingSeconds: 7 * 24 * 60 * 60},
		},
		Resets: result,
	}

	var output strings.Builder
	if err := RenderJSON(&output, report, Options{Location: time.UTC, Timezone: "Etc/UTC", Now: now, Color: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "secret") || strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("JSON leaked internal data or ANSI: %s", output.String())
	}
	golden, err := os.ReadFile("testdata/json_v1.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), string(golden); got != want {
		t.Fatalf("JSON schema output changed:\n--- got ---\n%s--- want ---\n%s", got, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["schema_version"]; got != float64(1) {
		t.Fatalf("schema_version = %#v", got)
	}
	if got := decoded["timezone"]; got != "Etc/UTC" {
		t.Fatalf("timezone = %#v", got)
	}
	limitData := decoded["limits"].(map[string]any)
	fiveHour := limitData["five_hour"].(map[string]any)
	if fiveHour["used_percent"] != float64(42) || fiveHour["remaining_percent"] != float64(58) || fiveHour["remaining_seconds"] != float64(4*60*60) || fiveHour["reset_due"] != false {
		t.Fatalf("five-hour limit = %#v", fiveHour)
	}
	credits := decoded["credits"].([]any)
	second := credits[1].(map[string]any)
	if second["expires_at"] != nil || second["remaining_seconds"] != nil || second["does_not_expire"] != true {
		t.Fatalf("non-expiring credit = %#v", second)
	}
}

func TestRenderJSONUnavailableLimitsAreNull(t *testing.T) {
	var output strings.Builder
	if err := RenderJSON(&output, Report{Resets: reset.Output{Credits: []reset.Expiration{}, Warnings: []reset.Warning{}}}, Options{Location: time.UTC, Now: time.Unix(0, 0)}); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Limits struct {
			FiveHour any `json:"five_hour"`
			Weekly   any `json:"weekly"`
		} `json:"limits"`
	}
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Limits.FiveHour != nil || decoded.Limits.Weekly != nil {
		t.Fatalf("limits = %#v", decoded.Limits)
	}
}

func TestRenderJSONKeepsPercentagesAndNullsUnknownResetData(t *testing.T) {
	report := Report{Limits: limits.Output{
		FiveHour: &limits.Window{UsedPercent: 37},
	}}
	var output strings.Builder
	if err := RenderJSON(&output, report, Options{Location: time.UTC, Now: time.Unix(0, 0)}); err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Limits struct {
			FiveHour *struct {
				UsedPercent      float64 `json:"used_percent"`
				RemainingPercent float64 `json:"remaining_percent"`
				ResetsAt         *string `json:"resets_at"`
				ResetsAtUnix     *int64  `json:"resets_at_unix"`
				RemainingSeconds *int64  `json:"remaining_seconds"`
				ResetDue         *bool   `json:"reset_due"`
			} `json:"five_hour"`
		} `json:"limits"`
	}
	if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	window := decoded.Limits.FiveHour
	if window == nil {
		t.Fatal("five-hour limit is null")
	}
	if window.UsedPercent != 37 || window.RemainingPercent != 63 {
		t.Fatalf("percentages = used:%v remaining:%v", window.UsedPercent, window.RemainingPercent)
	}
	if window.ResetsAt != nil || window.ResetsAtUnix != nil || window.RemainingSeconds != nil || window.ResetDue != nil {
		t.Fatalf("unknown reset data was not null: %#v", window)
	}
}

func TestRenderJSONLocationChangesStringsNotUnixValues(t *testing.T) {
	resetAt := time.Date(2026, time.July, 12, 5, 30, 0, 0, time.UTC)
	report := Report{Limits: limits.Output{FiveHour: &limits.Window{
		UsedPercent:      25,
		ResetsAt:         &resetAt,
		RemainingSeconds: 3600,
	}}}

	decode := func(location *time.Location) map[string]any {
		t.Helper()
		var output strings.Builder
		if err := RenderJSON(&output, report, Options{Location: location, Now: resetAt.Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(output.String()), &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded["limits"].(map[string]any)["five_hour"].(map[string]any)
	}

	local := decode(time.FixedZone("NZST", 12*60*60))
	utc := decode(time.UTC)
	if local["resets_at"] == utc["resets_at"] {
		t.Fatalf("localized reset strings unexpectedly equal: %q", local["resets_at"])
	}
	if local["resets_at_unix"] != utc["resets_at_unix"] || local["remaining_seconds"] != utc["remaining_seconds"] {
		t.Fatalf("absolute values changed: local=%#v utc=%#v", local, utc)
	}
}

func TestRenderHumanResetDue(t *testing.T) {
	resetAt := time.Date(2026, time.July, 12, 5, 30, 0, 0, time.UTC)
	report := Report{Limits: limits.Output{FiveHour: &limits.Window{
		UsedPercent: 100,
		ResetsAt:    &resetAt,
		ResetDue:    true,
	}}}

	var output, warnings strings.Builder
	if err := RenderHuman(&output, &warnings, report, Options{Location: time.UTC, Width: 80, Color: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "0%") || !strings.Contains(output.String(), ansiRed+"reset due"+ansiReset) {
		t.Fatalf("reset-due output = %q", output.String())
	}
}

func TestRemainingPercentIsClamped(t *testing.T) {
	tests := []struct {
		used float64
		want float64
	}{
		{used: -10, want: 100},
		{used: 0, want: 100},
		{used: 37.5, want: 62.5},
		{used: 100, want: 0},
		{used: 125, want: 0},
	}
	for _, test := range tests {
		if got := remainingPercent(test.used); got != test.want {
			t.Errorf("remainingPercent(%v) = %v, want %v", test.used, got, test.want)
		}
	}
}

func TestRenderErrorJSON(t *testing.T) {
	var output strings.Builder
	problem := Error{Code: "auth_missing", Message: "Unable to find Codex authentication.", Action: "Run codex login."}
	if err := RenderError(&output, problem, true); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "{\"error\":{\"code\":\"auth_missing\",\"message\":\"Unable to find Codex authentication.\",\"action\":\"Run codex login.\"}}\n"; got != want {
		t.Fatalf("RenderError() = %q, want %q", got, want)
	}
}

func TestRemainingTimeThresholds(t *testing.T) {
	expires := time.Unix(1, 0)
	tests := []struct {
		credit reset.Expiration
		want   string
	}{
		{credit: reset.Expiration{ExpiresAt: &expires, RemainingSeconds: 21*24*60*60 + 23*60*60}, want: "21d 23h"},
		{credit: reset.Expiration{ExpiresAt: &expires, RemainingSeconds: 4*60*60 + 12*60}, want: "4h 12m"},
		{credit: reset.Expiration{ExpiresAt: &expires, RemainingSeconds: 12*60 + 9}, want: "12m 9s"},
		{credit: reset.Expiration{ExpiresAt: &expires, RemainingSeconds: 42}, want: "42s"},
		{credit: reset.Expiration{ExpiresAt: &expires, Expired: true}, want: "expired"},
		{credit: reset.Expiration{DoesNotExpire: true}, want: "—"},
	}
	for _, test := range tests {
		if got := remainingTime(test.credit); got != test.want {
			t.Errorf("remainingTime(%#v) = %q, want %q", test.credit, got, test.want)
		}
	}
}

func TestUrgencyColorThresholds(t *testing.T) {
	tests := []struct {
		credit reset.Expiration
		want   string
	}{
		{credit: reset.Expiration{RemainingSeconds: 3599}, want: ansiRed},
		{credit: reset.Expiration{RemainingSeconds: 3600}, want: ansiYellow},
		{credit: reset.Expiration{RemainingSeconds: 86399}, want: ansiYellow},
		{credit: reset.Expiration{RemainingSeconds: 86400}, want: ""},
		{credit: reset.Expiration{Expired: true}, want: ansiRed},
		{credit: reset.Expiration{DoesNotExpire: true}, want: ""},
	}
	for _, test := range tests {
		if got := urgencyColor(test.credit); got != test.want {
			t.Errorf("urgencyColor(%#v) = %q, want %q", test.credit, got, test.want)
		}
	}
}

func TestExactTimeAcrossDSTBoundary(t *testing.T) {
	zone, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, time.April, 4, 13, 30, 0, 0, time.UTC)
	after := before.Add(time.Hour)

	if got, want := exactTime(reset.Expiration{ExpiresAt: &before}, zone), "Sun 05 Apr 2026 2:30:00 AM NZDT"; got != want {
		t.Fatalf("before transition = %q, want %q", got, want)
	}
	if got, want := exactTime(reset.Expiration{ExpiresAt: &after}, zone), "Sun 05 Apr 2026 2:30:00 AM NZST"; got != want {
		t.Fatalf("after transition = %q, want %q", got, want)
	}
}
