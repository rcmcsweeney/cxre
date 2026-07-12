package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testSecret = "CXRE_TEST_SECRET_DO_NOT_PRINT"

func TestMain(m *testing.M) {
	if os.Getenv("CXRE_CODEX_TEST_HELPER") == "1" {
		os.Exit(runHelper(os.Getenv("CXRE_CODEX_TEST_SCENARIO")))
	}
	os.Exit(m.Run())
}

func TestFetchSuccess(t *testing.T) {
	configureHelper(t, "success")

	result, err := fetch(context.Background(), fetchOptions{
		executable:    os.Args[0],
		clientVersion: "0.1.0-test",
		timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if result.AvailableCount != 2 || !result.DetailsProvided {
		t.Fatalf("unexpected result summary: %+v", result)
	}
	if len(result.Credits) != 2 {
		t.Fatalf("got %d credits, want 2", len(result.Credits))
	}

	first := result.Credits[0]
	if first.ID != "opaque-credit-1" || first.ResetType != "codexRateLimits" || first.Status != "available" {
		t.Fatalf("unexpected first credit: %+v", first)
	}
	if !first.GrantedAtProvided || first.GrantedAt == nil || first.GrantedAt.Unix() != 1781654400 {
		t.Fatalf("unexpected grantedAt: %+v", first)
	}
	if !first.ExpiresAtProvided || first.ExpiresAt == nil || first.ExpiresAt.Unix() != 1784246400 {
		t.Fatalf("unexpected expiresAt: %+v", first)
	}
	second := result.Credits[1]
	if !second.ExpiresAtProvided || second.ExpiresAt != nil {
		t.Fatalf("explicit null expiry was not preserved: %+v", second)
	}
}

func TestFetchCountOnly(t *testing.T) {
	configureHelper(t, "count_only")

	result, err := fetchHelper(t, time.Second)
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if result.AvailableCount != 3 || result.DetailsProvided || result.Credits != nil {
		t.Fatalf("unexpected count-only result: %+v", result)
	}
}

func TestFetchExplicitZeroWithEmptyDetails(t *testing.T) {
	configureHelper(t, "zero")

	result, err := fetchHelper(t, time.Second)
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if result.AvailableCount != 0 || !result.DetailsProvided || result.Credits == nil || len(result.Credits) != 0 {
		t.Fatalf("unexpected zero-credit result: %#v", result)
	}
}

func TestFetchPreservesCappedAndUnknownDetails(t *testing.T) {
	tests := []struct {
		name       string
		scenario   string
		wantCount  int
		wantRows   int
		wantStatus string
	}{
		{name: "capped", scenario: "capped", wantCount: 3, wantRows: 1, wantStatus: "available"},
		{name: "unknown status", scenario: "unknown_status", wantCount: 1, wantRows: 1, wantStatus: "futureStatus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureHelper(t, tt.scenario)
			result, err := fetchHelper(t, time.Second)
			if err != nil {
				t.Fatalf("fetch returned error: %v", err)
			}
			if result.AvailableCount != tt.wantCount || !result.DetailsProvided || len(result.Credits) != tt.wantRows {
				t.Fatalf("unexpected result shape: %+v", result)
			}
			if result.Credits[0].Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", result.Credits[0].Status, tt.wantStatus)
			}
		})
	}
}

func TestFetchPreservesMissingTimestampFields(t *testing.T) {
	configureHelper(t, "missing_timestamps")

	result, err := fetchHelper(t, time.Second)
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if len(result.Credits) != 1 {
		t.Fatalf("got %d credits, want 1", len(result.Credits))
	}
	credit := result.Credits[0]
	if credit.GrantedAtProvided || credit.GrantedAt != nil || credit.ExpiresAtProvided || credit.ExpiresAt != nil {
		t.Fatalf("missing timestamp fields were not preserved: %+v", credit)
	}
}

func TestFetchUsageWindows(t *testing.T) {
	t.Run("primary and secondary", func(t *testing.T) {
		configureHelper(t, "usage_primary_secondary")

		result, err := fetchHelper(t, time.Second)
		if err != nil {
			t.Fatalf("fetch returned error: %v", err)
		}
		assertUsageWindow(t, result.FiveHour, 17.5, 1783857600)
		assertUsageWindow(t, result.Weekly, 61, 1784376000)
	})

	t.Run("multi-bucket codex fallback", func(t *testing.T) {
		configureHelper(t, "usage_multi_fallback")

		result, err := fetchHelper(t, time.Second)
		if err != nil {
			t.Fatalf("fetch returned error: %v", err)
		}
		assertUsageWindow(t, result.FiveHour, 23, 1783858600)
		assertUsageWindow(t, result.Weekly, 72.25, 1784462400)
		if rendered := fmt.Sprintf("%+v", result); strings.Contains(rendered, testSecret) {
			t.Fatalf("result leaked an ignored protocol field: %s", rendered)
		}
	})

	t.Run("missing malformed and unrecognized windows are nonfatal", func(t *testing.T) {
		configureHelper(t, "usage_unknown_windows")

		result, err := fetchHelper(t, time.Second)
		if err != nil {
			t.Fatalf("fetch returned error: %v", err)
		}
		if result.FiveHour != nil || result.Weekly != nil {
			t.Fatalf("unexpected recognized usage windows: %+v", result)
		}
		if result.AvailableCount != 0 || !result.DetailsProvided {
			t.Fatalf("valid reset-credit data was not preserved: %+v", result)
		}
	})
}

func TestUsageWindowFallbackFillsOnlyMissingLegacyWindow(t *testing.T) {
	result := Result{}
	parseUsageLimits(
		&result,
		json.RawMessage(`{
			"primary":{"usedPercent":11,"windowDurationMins":300,"resetsAt":1783857600},
			"secondary":{"usedPercent":2,"windowDurationMins":60,"resetsAt":1783857700}
		}`),
		json.RawMessage(`{
			"codex":{
				"primary":{"usedPercent":99,"windowDurationMins":300,"resetsAt":1783857800},
				"secondary":{"usedPercent":22,"windowDurationMins":10080,"resetsAt":1784462400}
			}
		}`),
	)

	assertUsageWindow(t, result.FiveHour, 11, 1783857600)
	assertUsageWindow(t, result.Weekly, 22, 1784462400)
}

func TestUsageWindowFallbackFillsResetWithoutReplacingLegacyPercent(t *testing.T) {
	result := Result{}
	parseUsageLimits(
		&result,
		json.RawMessage(`{
			"primary":{"usedPercent":11,"windowDurationMins":300,"resetsAt":null},
			"secondary":{"usedPercent":22,"windowDurationMins":10080,"resetsAt":"malformed"}
		}`),
		json.RawMessage(`{
			"codex":{
				"primary":{"usedPercent":99,"windowDurationMins":300,"resetsAt":1783857800},
				"secondary":{"usedPercent":88,"windowDurationMins":10080,"resetsAt":1784462400}
			}
		}`),
	)

	assertUsageWindow(t, result.FiveHour, 11, 1783857800)
	assertUsageWindow(t, result.Weekly, 22, 1784462400)
}

func TestUsageWindowsPreservePercentWithoutResetTime(t *testing.T) {
	result := Result{}
	parseUsageLimits(
		&result,
		json.RawMessage(`{
			"primary":{"usedPercent":33.5,"windowDurationMins":300,"resetsAt":null},
			"secondary":{"usedPercent":44,"windowDurationMins":10080}
		}`),
		nil,
	)

	assertUsageWindowWithoutReset(t, result.FiveHour, 33.5)
	assertUsageWindowWithoutReset(t, result.Weekly, 44)
}

func TestUsageWindowDurationTolerance(t *testing.T) {
	tests := []struct {
		name     string
		duration int64
		wantKind usageWindowKind
	}{
		{name: "five hour lower boundary", duration: 285, wantKind: usageWindowFiveHour},
		{name: "five hour upper boundary", duration: 315, wantKind: usageWindowFiveHour},
		{name: "below five hour tolerance", duration: 284, wantKind: usageWindowUnknown},
		{name: "above five hour tolerance", duration: 316, wantKind: usageWindowUnknown},
		{name: "weekly lower boundary", duration: 9576, wantKind: usageWindowWeekly},
		{name: "weekly upper boundary", duration: 10584, wantKind: usageWindowWeekly},
		{name: "below weekly tolerance", duration: 9575, wantKind: usageWindowUnknown},
		{name: "above weekly tolerance", duration: 10585, wantKind: usageWindowUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(fmt.Sprintf(
				`{"usedPercent":123.75,"windowDurationMins":%d,"resetsAt":%q}`,
				tt.duration,
				testSecret,
			))
			window, kind, ok := parseUsageWindow(raw)
			if tt.wantKind == usageWindowUnknown {
				if ok || kind != usageWindowUnknown || window != nil {
					t.Fatalf("duration %d was classified as %v", tt.duration, kind)
				}
				return
			}
			if !ok || kind != tt.wantKind || window == nil {
				t.Fatalf("duration %d = (%+v, %v, %v), want kind %v", tt.duration, window, kind, ok, tt.wantKind)
			}
			if window.UsedPercent != 123.75 || window.ResetsAt != nil {
				t.Fatalf("optional reset or permissive percentage was not preserved: %+v", window)
			}
			if rendered := fmt.Sprintf("%+v", window); strings.Contains(rendered, testSecret) {
				t.Fatalf("window leaked malformed reset data: %s", rendered)
			}
		})
	}
}

func TestFetchAuthenticationModes(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		wantCode Code
	}{
		{name: "missing", scenario: "auth_missing", wantCode: CodeAuthMissing},
		{name: "api key", scenario: "auth_apikey", wantCode: CodeUnsupportedAuth},
		{name: "Bedrock", scenario: "auth_bedrock", wantCode: CodeUnsupportedAuth},
		{name: "unknown", scenario: "auth_unknown", wantCode: CodeUnsupportedAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureHelper(t, tt.scenario)
			_, err := fetchHelper(t, time.Second)
			assertCode(t, err, tt.wantCode)
		})
	}
}

func TestFetchProtocolErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		wantCode Code
	}{
		{name: "method unavailable", scenario: "rpc_old", wantCode: CodeCodexTooOld},
		{name: "RPC authentication", scenario: "rpc_auth", wantCode: CodeAuthMissing},
		{name: "RPC network", scenario: "rpc_network", wantCode: CodeNetwork},
		{name: "RPC timeout", scenario: "rpc_timeout", wantCode: CodeTimeout},
		{name: "null summary", scenario: "null_summary", wantCode: CodeProtocol},
		{name: "missing summary", scenario: "missing_summary", wantCode: CodeCodexTooOld},
		{name: "missing count", scenario: "missing_count", wantCode: CodeProtocol},
		{name: "negative count", scenario: "negative_count", wantCode: CodeProtocol},
		{name: "malformed message", scenario: "malformed", wantCode: CodeProtocol},
		{name: "wrong response id", scenario: "wrong_id", wantCode: CodeProtocol},
		{name: "process failure", scenario: "exit", wantCode: CodeProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configureHelper(t, tt.scenario)
			_, err := fetchHelper(t, time.Second)
			assertCode(t, err, tt.wantCode)
			assertSanitized(t, err)
		})
	}
}

func TestFetchTimeout(t *testing.T) {
	configureHelper(t, "timeout")

	started := time.Now()
	_, err := fetchHelper(t, 30*time.Millisecond)
	assertCode(t, err, CodeTimeout)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
	assertSanitized(t, err)
}

func TestFetchCancellationStopsChild(t *testing.T) {
	configureHelper(t, "timeout")
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(30*time.Millisecond, cancel)
	defer timer.Stop()

	started := time.Now()
	_, err := fetch(ctx, fetchOptions{
		executable:    os.Args[0],
		clientVersion: "0.1.0-test",
		timeout:       time.Second,
	})
	assertCode(t, err, CodeTimeout)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellation cleanup took %s", elapsed)
	}
}

func TestFetchLingeringChildIsBounded(t *testing.T) {
	configureHelper(t, "linger")

	started := time.Now()
	// Keep the request deadline well beyond the assertion so the cleanup's
	// grace-and-kill fallback, not CommandContext's deadline, proves the bound.
	result, err := fetchHelper(t, 5*time.Second)
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if result.AvailableCount != 0 || !result.DetailsProvided {
		t.Fatalf("unexpected result: %+v", result)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("lingering child cleanup took %s", elapsed)
	}
}

func TestFetchExecutableNotFound(t *testing.T) {
	_, err := fetch(context.Background(), fetchOptions{
		executable:    t.TempDir() + string(os.PathSeparator) + "missing-codex",
		clientVersion: "test",
		timeout:       time.Second,
	})
	assertCode(t, err, CodeCodexNotFound)
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		envSet      bool
		pathValue   string
		pathErr     error
		want        string
		wantCode    Code
		lookPathHit bool
	}{
		{name: "environment wins", envValue: "/custom/codex", envSet: true, pathValue: "/path/codex", want: "/custom/codex"},
		{name: "PATH fallback", pathValue: "/path/codex", want: "/path/codex", lookPathHit: true},
		{name: "explicit empty is invalid", envSet: true, wantCode: CodeCodexNotFound},
		{name: "not on PATH", pathErr: exec.ErrNotFound, wantCode: CodeCodexNotFound, lookPathHit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			got, err := resolvePath(
				func(key string) (string, bool) {
					if key != "CXRE_CODEX" {
						t.Fatalf("unexpected environment lookup: %q", key)
					}
					return tt.envValue, tt.envSet
				},
				func(name string) (string, error) {
					called = true
					if name != "codex" {
						t.Fatalf("unexpected path lookup: %q", name)
					}
					return tt.pathValue, tt.pathErr
				},
			)
			if tt.wantCode != "" {
				assertCode(t, err, tt.wantCode)
			} else if err != nil {
				t.Fatalf("resolvePath returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolvePath = %q, want %q", got, tt.want)
			}
			if called != tt.lookPathHit {
				t.Fatalf("lookPath called = %v, want %v", called, tt.lookPathHit)
			}
		})
	}
}

func TestErrorContract(t *testing.T) {
	for _, code := range []Code{
		CodeCodexNotFound,
		CodeAuthMissing,
		CodeUnsupportedAuth,
		CodeCodexTooOld,
		CodeTimeout,
		CodeNetwork,
		CodeProtocol,
	} {
		err := failure(code, errors.New(testSecret))
		if err.Code != code || err.Message == "" || err.Action == "" {
			t.Fatalf("incomplete error for %q: %+v", code, err)
		}
		if CodeOf(err) != code {
			t.Fatalf("CodeOf(%q) = %q", code, CodeOf(err))
		}
		assertSanitized(t, err)
	}
	if CodeOf(errors.New("foreign")) != CodeProtocol {
		t.Fatal("foreign errors must map to protocol")
	}
}

func TestLimitedCaptureIsBounded(t *testing.T) {
	var capture limitedCapture
	input := strings.Repeat("x", maxCapturedStderr+1024)
	n, err := capture.Write([]byte(input))
	if err != nil || n != len(input) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(input))
	}
	if got := len(capture.String()); got != maxCapturedStderr {
		t.Fatalf("captured %d bytes, want %d", got, maxCapturedStderr)
	}
}

func TestStderrFailureClassification(t *testing.T) {
	tests := []struct {
		name string
		text string
		code Code
	}{
		{name: "network", text: testSecret + " failed to connect", code: CodeNetwork},
		{name: "timeout", text: testSecret + " deadline exceeded", code: CodeTimeout},
		{name: "old CLI command", text: testSecret + " unrecognized subcommand 'app-server'", code: CodeCodexTooOld},
		{name: "old CLI flag", text: testSecret + " unexpected argument '--stdio'", code: CodeCodexTooOld},
		{name: "unknown", text: testSecret, code: CodeProtocol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capture limitedCapture
			_, _ = capture.Write([]byte(tt.text))
			transport := jsonlTransport{ctx: context.Background(), stderr: &capture}
			err := transport.ioFailure(errors.New(testSecret))
			assertCode(t, err, tt.code)
			assertSanitized(t, err)
		})
	}
}

// TestLiveIntegration is deliberately opt-in: it uses the caller's existing
// Codex sign-in but records and prints no account data, credit IDs, or times.
func TestLiveIntegration(t *testing.T) {
	if os.Getenv("CXRE_INTEGRATION") != "1" {
		t.Skip("set CXRE_INTEGRATION=1 to test against an existing Codex sign-in")
	}

	result, err := Fetch(context.Background(), "integration-test")
	if err != nil {
		// Error is the package's sanitized, allowlisted user-facing value.
		t.Fatalf("live app-server request failed: %v", err)
	}
	if result.AvailableCount < 0 {
		t.Fatal("live app-server returned a negative available count")
	}
	if !result.DetailsProvided && len(result.Credits) != 0 {
		t.Fatal("live app-server returned rows while marking details unavailable")
	}
	if len(result.Credits) > result.AvailableCount {
		t.Fatal("live app-server returned more rows than its authoritative count")
	}
	for _, credit := range result.Credits {
		if credit.GrantedAt != nil && !credit.GrantedAtProvided {
			t.Fatal("live app-server returned a granted time without its presence marker")
		}
		if credit.ExpiresAt != nil && !credit.ExpiresAtProvided {
			t.Fatal("live app-server returned an expiry time without its presence marker")
		}
	}
	for _, window := range []*UsageWindow{result.FiveHour, result.Weekly} {
		if window != nil && window.ResetsAt != nil && window.ResetsAt.Location() != time.UTC {
			t.Fatal("live app-server returned a usage reset outside UTC")
		}
	}
}

func fetchHelper(t *testing.T, timeout time.Duration) (Result, error) {
	t.Helper()
	return fetch(context.Background(), fetchOptions{
		executable:    os.Args[0],
		clientVersion: "0.1.0-test",
		timeout:       timeout,
	})
}

func configureHelper(t *testing.T, scenario string) {
	t.Helper()
	t.Setenv("CXRE_CODEX_TEST_HELPER", "1")
	t.Setenv("CXRE_CODEX_TEST_SCENARIO", scenario)
}

func assertCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want code %q", want)
	}
	if got := CodeOf(err); got != want {
		t.Fatalf("CodeOf(error) = %q, want %q (error: %v)", got, want, err)
	}
	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("error has type %T, want *codex.Error", err)
	}
}

func assertSanitized(t *testing.T, err error) {
	t.Helper()
	for _, rendered := range []string{
		err.Error(),
		fmt.Sprintf("%v", err),
		fmt.Sprintf("%+v", err),
		fmt.Sprintf("%#v", err),
	} {
		if strings.Contains(rendered, testSecret) {
			t.Fatalf("error leaked sentinel secret: %s", rendered)
		}
	}
}

func assertUsageWindow(t *testing.T, got *UsageWindow, wantPercent float64, wantReset int64) {
	t.Helper()
	if got == nil {
		t.Fatal("usage window is nil")
	}
	if got.UsedPercent != wantPercent {
		t.Fatalf("used percent = %v, want %v", got.UsedPercent, wantPercent)
	}
	if got.ResetsAt == nil {
		t.Fatal("reset time is nil")
	}
	if got.ResetsAt.Location() != time.UTC || got.ResetsAt.Unix() != wantReset {
		t.Fatalf("reset = %v, want Unix %d in UTC", got.ResetsAt, wantReset)
	}
}

func assertUsageWindowWithoutReset(t *testing.T, got *UsageWindow, wantPercent float64) {
	t.Helper()
	if got == nil {
		t.Fatal("usage window is nil")
	}
	if got.UsedPercent != wantPercent || got.ResetsAt != nil {
		t.Fatalf("usage window = %+v, want percent %v without reset", got, wantPercent)
	}
}

func runHelper(scenario string) int {
	if !reflect.DeepEqual(os.Args[1:], []string{"app-server", "--stdio"}) {
		return 90
	}
	if scenario == "exit" {
		_, _ = fmt.Fprintln(os.Stderr, testSecret)
		return 91
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	request, ok := readHelperRequest(scanner)
	if !ok || request.Method != "initialize" || request.ID == nil || *request.ID != 0 {
		return 92
	}
	var init initializeParams
	if err := json.Unmarshal(request.Params, &init); err != nil ||
		init.ClientInfo.Name != "cxre" || init.ClientInfo.Title != "CXRE" || init.ClientInfo.Version != "0.1.0-test" {
		return 93
	}
	var initObject map[string]json.RawMessage
	if err := json.Unmarshal(request.Params, &initObject); err != nil {
		return 94
	}
	if _, optedIn := initObject["capabilities"]; optedIn {
		return 95
	}

	if scenario == "timeout" {
		time.Sleep(5 * time.Second)
		return 96
	}
	if scenario == "malformed" {
		_, _ = fmt.Fprintln(os.Stdout, "not-json-"+testSecret)
		_, _ = fmt.Fprintln(os.Stderr, testSecret)
		return 0
	}
	if scenario == "wrong_id" {
		_ = encoder.Encode(map[string]any{"id": 999, "result": map[string]any{}})
		return 0
	}

	_ = encoder.Encode(map[string]any{"method": "account/updated", "params": map[string]any{"authMode": "chatgpt"}})
	_ = encoder.Encode(map[string]any{"id": 0, "result": map[string]any{"userAgent": "codex-test"}})
	request, ok = readHelperRequest(scanner)
	if !ok || request.Method != "initialized" || request.ID != nil || string(request.Params) != "{}" {
		return 97
	}
	request, ok = readHelperRequest(scanner)
	if !ok || request.Method != "account/read" || request.ID == nil || *request.ID != 1 {
		return 98
	}
	var accountParams accountReadParams
	if err := json.Unmarshal(request.Params, &accountParams); err != nil || accountParams.RefreshToken {
		return 99
	}

	switch scenario {
	case "auth_missing":
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"account": nil, "requiresOpenaiAuth": true}})
		return 0
	case "auth_apikey":
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"account": map[string]any{"type": "apiKey"}}})
		return 0
	case "auth_bedrock":
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"account": map[string]any{"type": "amazonBedrock"}}})
		return 0
	case "auth_unknown":
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"account": map[string]any{"type": "futureAuth"}}})
		return 0
	default:
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{
			"account": map[string]any{"type": "chatgpt", "email": testSecret},
		}})
	}

	request, ok = readHelperRequest(scanner)
	if !ok || request.Method != "account/rateLimits/read" || request.ID == nil || *request.ID != 2 || len(request.Params) != 0 {
		return 100
	}
	_ = encoder.Encode(map[string]any{"method": "account/rateLimits/updated", "params": map[string]any{"ignored": true}})

	switch scenario {
	case "success":
		_, _ = fmt.Fprintln(os.Stderr, testSecret)
		return writeRateResult(encoder, map[string]any{
			"availableCount": 2,
			"credits": []any{
				map[string]any{"id": "opaque-credit-1", "resetType": "codexRateLimits", "status": "available", "grantedAt": int64(1781654400), "expiresAt": int64(1784246400), "title": testSecret},
				map[string]any{"id": "opaque-credit-2", "resetType": "codexRateLimits", "status": "available", "grantedAt": int64(1781654500), "expiresAt": nil},
			},
		})
	case "count_only":
		return writeRateResult(encoder, map[string]any{"availableCount": 3, "credits": nil})
	case "zero":
		return writeRateResult(encoder, map[string]any{"availableCount": 0, "credits": []any{}})
	case "linger":
		if code := writeRateResult(encoder, map[string]any{"availableCount": 0, "credits": []any{}}); code != 0 {
			return code
		}
		_, _ = fmt.Fprintln(os.Stderr, testSecret)
		time.Sleep(5 * time.Second)
		return 0
	case "missing_timestamps":
		return writeRateResult(encoder, map[string]any{
			"availableCount": 1,
			"credits":        []any{map[string]any{"id": "opaque", "resetType": "codexRateLimits", "status": "available"}},
		})
	case "capped":
		return writeRateResult(encoder, map[string]any{
			"availableCount": 3,
			"credits": []any{map[string]any{
				"id": "opaque", "resetType": "codexRateLimits", "status": "available", "grantedAt": int64(1781654400), "expiresAt": int64(1784246400),
			}},
		})
	case "unknown_status":
		return writeRateResult(encoder, map[string]any{
			"availableCount": 1,
			"credits": []any{map[string]any{
				"id": "opaque", "resetType": "codexRateLimits", "status": "futureStatus", "grantedAt": int64(1781654400), "expiresAt": int64(1784246400),
			}},
		})
	case "usage_primary_secondary":
		return writeFullRateResult(encoder, map[string]any{
			"rateLimits": map[string]any{
				"limitId": "codex",
				"primary": map[string]any{
					"usedPercent": 17.5, "windowDurationMins": 300, "resetsAt": int64(1783857600),
				},
				"secondary": map[string]any{
					"usedPercent": 61, "windowDurationMins": 10080, "resetsAt": int64(1784376000),
				},
			},
			"rateLimitResetCredits": map[string]any{"availableCount": 0, "credits": []any{}},
		})
	case "usage_multi_fallback":
		return writeFullRateResult(encoder, map[string]any{
			"rateLimits": nil,
			"rateLimitsByLimitId": map[string]any{
				"codex": map[string]any{
					"limitId":   "codex",
					"limitName": testSecret,
					"primary": map[string]any{
						"usedPercent": 23, "windowDurationMins": 300, "resetsAt": int64(1783858600), "private": testSecret,
					},
					"secondary": map[string]any{
						"usedPercent": 72.25, "windowDurationMins": 10080, "resetsAt": int64(1784462400),
					},
				},
				"codex_other": map[string]any{
					"limitName": testSecret,
					"primary": map[string]any{
						"usedPercent": 100, "windowDurationMins": 300, "resetsAt": int64(1),
					},
				},
			},
			"rateLimitResetCredits": map[string]any{"availableCount": 0, "credits": []any{}},
		})
	case "usage_unknown_windows":
		return writeFullRateResult(encoder, map[string]any{
			"rateLimits": map[string]any{
				"primary": map[string]any{
					"usedPercent": 40, "windowDurationMins": 15, "resetsAt": int64(1783857600),
				},
				"secondary": map[string]any{
					"usedPercent": testSecret, "windowDurationMins": 300, "resetsAt": nil,
				},
			},
			"rateLimitsByLimitId": map[string]any{
				"codex_other": map[string]any{
					"primary": map[string]any{
						"usedPercent": 55, "windowDurationMins": 10080, "resetsAt": int64(1784462400),
					},
				},
			},
			"rateLimitResetCredits": map[string]any{"availableCount": 0, "credits": []any{}},
		})
	case "rpc_old":
		_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"code": -32601, "message": testSecret + " method not found"}})
	case "rpc_auth":
		_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"code": -32000, "message": testSecret + " authentication required"}})
	case "rpc_network":
		_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"code": -32000, "message": testSecret + " network unavailable"}})
	case "rpc_timeout":
		_ = encoder.Encode(map[string]any{"id": 2, "error": map[string]any{"code": -32000, "message": testSecret + " request timed out"}})
	case "null_summary":
		_ = encoder.Encode(map[string]any{"id": 2, "result": map[string]any{"rateLimitResetCredits": nil}})
	case "missing_summary":
		_ = encoder.Encode(map[string]any{"id": 2, "result": map[string]any{"rateLimits": map[string]any{}}})
	case "missing_count":
		return writeRateResult(encoder, map[string]any{"credits": []any{}})
	case "negative_count":
		return writeRateResult(encoder, map[string]any{"availableCount": -1, "credits": []any{}})
	default:
		return 101
	}
	return 0
}

type helperRequest struct {
	Method string          `json:"method"`
	ID     *int64          `json:"id"`
	Params json.RawMessage `json:"params"`
}

func readHelperRequest(scanner *bufio.Scanner) (helperRequest, bool) {
	if !scanner.Scan() {
		return helperRequest{}, false
	}
	var request helperRequest
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		return helperRequest{}, false
	}
	return request, true
}

func writeRateResult(encoder *json.Encoder, resetCredits map[string]any) int {
	return writeFullRateResult(encoder, map[string]any{"rateLimitResetCredits": resetCredits})
}

func writeFullRateResult(encoder *json.Encoder, result map[string]any) int {
	if err := encoder.Encode(map[string]any{
		"id":     2,
		"result": result,
	}); err != nil {
		return 102
	}
	return 0
}
