package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rcmcsweeney/cxre/internal/codex"
	"github.com/rcmcsweeney/cxre/internal/presentation"
)

func TestInterruptIsSilentAndRestoresDefaultBeforeCancellation(t *testing.T) {
	testSignalExit(t, os.Interrupt, 130)
}

func TestInterruptAfterFetchDiscardsStagedSuccess(t *testing.T) {
	nowStarted := make(chan struct{})
	releaseNow := make(chan struct{})
	deps := testDependencies()
	deps.now = func() time.Time {
		close(nowStarted)
		<-releaseNow
		return time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	}

	signals := make(chan os.Signal, 1)
	type result struct {
		exit   int
		stdout string
		stderr string
	}
	finished := make(chan result, 1)
	go func() {
		var stdout, stderr strings.Builder
		exit := runWithSignals(nil, &stdout, &stderr, deps, signals, func() {})
		finished <- result{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
	}()

	select {
	case <-nowStarted:
	case <-time.After(time.Second):
		t.Fatal("normalization did not start")
	}
	signals <- os.Interrupt
	close(releaseNow)

	select {
	case got := <-finished:
		if got.exit != 130 || got.stdout != "" || got.stderr != "" {
			t.Fatalf("exit=%d stdout=%q stderr=%q, want silent exit 130", got.exit, got.stdout, got.stderr)
		}
	case <-time.After(time.Second):
		t.Fatal("signal did not stop the command")
	}
}

func TestRealTimeoutStillRendersAnError(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{}, &codex.Error{
			Code:    codex.CodeTimeout,
			Message: "Codex did not respond in time.",
			Action:  "Try again.",
		}
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"timeout"`) {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunWithSignalsStopsWatcherAfterNormalCompletion(t *testing.T) {
	signals := make(chan os.Signal, 1)
	stopped := make(chan struct{})
	var stdout, stderr strings.Builder
	code := runWithSignals(
		[]string{"--help"},
		&stdout,
		&stderr,
		testDependencies(),
		signals,
		func() { close(stopped) },
	)
	if code != 0 || !strings.Contains(stdout.String(), "Usage:") || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	select {
	case <-stopped:
	default:
		t.Fatal("signal delivery was not stopped after normal completion")
	}
}

func TestRunWithSignalsReportsStagedCommitFailure(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "human", args: []string{"--help"}, want: "Unable to write CXRE output.\n"},
		{name: "JSON", args: []string{"--json"}, want: "{\"error\":{\"code\":\"output\",\"message\":\"Unable to write CXRE output.\"}}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr strings.Builder
			code := runWithSignals(
				test.args,
				failingWriter{},
				&stderr,
				testDependencies(),
				make(chan os.Signal),
				func() {},
			)
			if code != 1 || stderr.String() != test.want {
				t.Fatalf("exit=%d stderr=%q, want exit 1 and %q", code, stderr.String(), test.want)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestHelpAndVersionDoNotFetch(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		t.Fatal("fetch called")
		return codex.Result{}, nil
	}

	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--help"}, want: "Usage:"},
		{args: []string{"--version"}, want: "cxre dev\n"},
	} {
		var stdout, stderr strings.Builder
		if code := run(context.Background(), test.args, &stdout, &stderr, deps); code != 0 {
			t.Fatalf("run(%v) exit = %d, stderr = %s", test.args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), test.want) {
			t.Fatalf("run(%v) stdout = %q, want to contain %q", test.args, stdout.String(), test.want)
		}
	}
}

func TestUsageError(t *testing.T) {
	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"status"}, &stdout, &stderr, testDependencies())
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `unknown command "status"`) || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestUsageErrorHonorsJSONRegardlessOfOrder(t *testing.T) {
	for _, args := range [][]string{{"--json", "--bad"}, {"--bad", "--json"}} {
		var stdout, stderr strings.Builder
		code := run(context.Background(), args, &stdout, &stderr, testDependencies())
		if code != 2 {
			t.Fatalf("run(%v) exit = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"usage"`) || strings.Contains(stderr.String(), "Usage:") {
			t.Fatalf("run(%v) stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestJSONSuccessUTC(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	expires := now.Add(4*time.Hour + 12*time.Minute)
	fiveHourReset := now.Add(2*time.Hour + 30*time.Minute)
	weeklyReset := now.Add(5*24*time.Hour + 19*time.Hour)
	deps := testDependencies()
	deps.now = func() time.Time { return now }
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{
			AvailableCount:  1,
			DetailsProvided: true,
			FiveHour:        &codex.UsageWindow{UsedPercent: 37, ResetsAt: &fiveHourReset},
			Weekly:          &codex.UsageWindow{UsedPercent: 61, ResetsAt: &weeklyReset},
			Credits: []codex.Credit{{
				ID:                "opaque-secret",
				Status:            "available",
				ExpiresAt:         &expires,
				ExpiresAtProvided: true,
			}},
		}, nil
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json", "--utc"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	for _, expected := range []string{
		`"schema_version": 1`,
		`"timezone": "UTC"`,
		`"five_hour": {`,
		`"used_percent": 37`,
		`"weekly": {`,
		`"remaining_seconds": 15120`,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("JSON missing %s:\n%s", expected, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "opaque-secret") || stderr.Len() != 0 {
		t.Fatalf("leak or stderr: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestJSONKeepsLimitPercentageWithoutResetTime(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{
			AvailableCount:  0,
			DetailsProvided: true,
			FiveHour:        &codex.UsageWindow{UsedPercent: 37},
			Credits:         []codex.Credit{},
		}, nil
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	for _, expected := range []string{
		`"five_hour": {`,
		`"used_percent": 37`,
		`"remaining_percent": 63`,
		`"resets_at": null`,
		`"remaining_seconds": null`,
		`"reset_due": null`,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("JSON missing %s:\n%s", expected, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestMissingExpiryFieldBecomesPartial(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{
			AvailableCount:  1,
			DetailsProvided: true,
			Credits:         []codex.Credit{{ID: "id", Status: "available"}},
		}, nil
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"complete": false`) || !strings.Contains(stdout.String(), `"missing_count": 1`) {
		t.Fatalf("partial response not represented: %s", stdout.String())
	}
}

func TestMissingExpiryFieldCannotHideContradictoryZeroCount(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{
			AvailableCount:  0,
			DetailsProvided: true,
			Credits:         []codex.Credit{{ID: "id", Status: "available"}},
		}, nil
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"complete": false`) ||
		!strings.Contains(stdout.String(), `"inconsistent_reset_credit_details"`) {
		t.Fatalf("contradictory response not represented: %s", stdout.String())
	}
}

func TestSanitizedCodexError(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{}, &codex.Error{
			Code:    "auth_missing",
			Message: "Unable to find Codex authentication.",
			Action:  "Run codex login.",
		}
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"auth_missing"`) {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestNewErrorJSONContracts(t *testing.T) {
	tests := []struct {
		name    string
		problem *codex.Error
		want    string
	}{
		{
			name: "invalid selected executable",
			problem: &codex.Error{
				Code:    codex.CodeCodexInvalidExecutable,
				Message: "CXRE cannot start the selected Codex CLI.",
				Action:  "Correct or unset `CXRE_CODEX`, or reinstall Codex, then run `cxre` again.",
			},
			want: "{\"error\":{\"code\":\"codex_invalid_executable\",\"message\":\"CXRE cannot start the selected Codex CLI.\",\"action\":\"Correct or unset `CXRE_CODEX`, or reinstall Codex, then run `cxre` again.\"}}\n",
		},
		{
			name: "reset credits unavailable",
			problem: &codex.Error{
				Code:    codex.CodeResetCreditsUnavailable,
				Message: "Codex did not provide reset-credit information for this account.",
				Action:  "Try again later. If this continues, reset credits may not be available for your ChatGPT plan or workspace.",
			},
			want: "{\"error\":{\"code\":\"reset_credits_unavailable\",\"message\":\"Codex did not provide reset-credit information for this account.\",\"action\":\"Try again later. If this continues, reset credits may not be available for your ChatGPT plan or workspace.\"}}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := testDependencies()
			deps.fetch = func(context.Context, string) (codex.Result, error) {
				return codex.Result{}, test.problem
			}

			var stdout, stderr strings.Builder
			if code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps); code != 1 {
				t.Fatalf("exit = %d, want 1", code)
			}
			if stdout.Len() != 0 || stderr.String() != test.want {
				t.Fatalf("stdout=%q stderr=%q, want stderr %q", stdout.String(), stderr.String(), test.want)
			}
		})
	}
}

func TestUnknownErrorIsNotEchoed(t *testing.T) {
	deps := testDependencies()
	deps.fetch = func(context.Context, string) (codex.Result, error) {
		return codex.Result{}, errors.New("SECRET_TOKEN_DO_NOT_PRINT")
	}

	var stdout, stderr strings.Builder
	code := run(context.Background(), nil, &stdout, &stderr, deps)
	if code != 1 || strings.Contains(stderr.String(), "SECRET_TOKEN_DO_NOT_PRINT") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}

func TestTerminalCapabilities(t *testing.T) {
	env := map[string]string{"LANG": "en_NZ.UTF-8"}
	deps := testDependencies()
	deps.lookupEnv = func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
	if !supportsColor(deps) || !supportsUnicode(deps) {
		t.Fatal("expected color and Unicode")
	}
	env["NO_COLOR"] = ""
	if supportsColor(deps) {
		t.Fatal("NO_COLOR presence should disable color")
	}
	env["LANG"] = "C"
	if supportsUnicode(deps) {
		t.Fatal("C locale should disable Unicode")
	}
	env["LANG"] = "en_NZ.UTF-8"
	env["TERM"] = "dumb"
	if supportsUnicode(deps) {
		t.Fatal("TERM=dumb should disable Unicode")
	}
}

func TestDecorateTerminalTreatsWarningStreamIndependently(t *testing.T) {
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		_ = stderrRead.Close()
		_ = stderrWrite.Close()
	})

	stdoutFD := int(stdoutWrite.Fd())
	stderrFD := int(stderrWrite.Fd())
	deps := testDependencies()
	deps.lookupEnv = func(key string) (string, bool) {
		if key == "LANG" {
			return "en_NZ.UTF-8", true
		}
		return "", false
	}
	deps.termSize = func(fd int) (int, int, error) {
		if fd != stdoutFD {
			t.Fatalf("termSize called for fd %d, want stdout fd %d", fd, stdoutFD)
		}
		return 72, 24, nil
	}

	t.Run("redirected stderr remains plain", func(t *testing.T) {
		deps.isTerminal = func(fd int) bool { return fd == stdoutFD }
		var options presentation.Options
		decorateTerminal(stdoutWrite, stderrWrite, &options, deps)
		if !options.Color || !options.Unicode || options.Width != 72 || options.WarningColor {
			t.Fatalf("unexpected options: %+v", options)
		}
	})

	t.Run("terminal stderr may use color independently", func(t *testing.T) {
		deps.isTerminal = func(fd int) bool { return fd == stderrFD }
		var options presentation.Options
		decorateTerminal(stdoutWrite, stderrWrite, &options, deps)
		if options.Color || options.Unicode || options.Width != 0 || !options.WarningColor {
			t.Fatalf("unexpected options: %+v", options)
		}
	})

	t.Run("non-file stderr remains plain", func(t *testing.T) {
		deps.isTerminal = func(int) bool { return true }
		var options presentation.Options
		decorateTerminal(stdoutWrite, &strings.Builder{}, &options, deps)
		if !options.Color || options.WarningColor {
			t.Fatalf("unexpected options: %+v", options)
		}
	})
}

func TestTimezoneName(t *testing.T) {
	local := time.FixedZone("Local", 12*60*60)
	now := time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		location *time.Location
		env      map[string]string
		link     string
		want     string
	}{
		{name: "UTC", location: time.UTC, want: "UTC"},
		{name: "named location", location: time.FixedZone("NZST", 12*60*60), want: "NZST"},
		{name: "TZ environment", location: local, env: map[string]string{"TZ": "Pacific/Auckland"}, want: "Pacific/Auckland"},
		{name: "zoneinfo symlink", location: local, link: "/var/db/timezone/zoneinfo/Pacific/Auckland", want: "Pacific/Auckland"},
		{name: "abbreviation fallback", location: local, want: "Local"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := testDependencies()
			deps.lookupEnv = func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			}
			deps.readlink = func(string) (string, error) {
				if test.link == "" {
					return "", errors.New("not found")
				}
				return test.link, nil
			}
			if got := timezoneName(test.location, now, deps); got != test.want {
				t.Fatalf("timezoneName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestJSONLocalTimezoneName(t *testing.T) {
	zone := time.FixedZone("Local", 12*60*60)
	deps := testDependencies()
	deps.local = zone
	deps.lookupEnv = func(key string) (string, bool) {
		if key == "TZ" {
			return "Pacific/Auckland", true
		}
		return "", false
	}

	var stdout, stderr strings.Builder
	if code := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"timezone": "Pacific/Auckland"`) {
		t.Fatalf("unexpected JSON timezone: %s", stdout.String())
	}
}

func testDependencies() dependencies {
	return dependencies{
		fetch: func(context.Context, string) (codex.Result, error) {
			return codex.Result{AvailableCount: 0, DetailsProvided: true}, nil
		},
		now:        func() time.Time { return time.Date(2026, time.July, 12, 1, 0, 0, 0, time.UTC) },
		lookupEnv:  func(string) (string, bool) { return "", false },
		isTerminal: func(int) bool { return false },
		termSize:   func(int) (int, int, error) { return 80, 24, nil },
		readlink:   func(string) (string, error) { return "", errors.New("not found") },
		local:      time.UTC,
		goos:       "darwin",
	}
}

func testSignalExit(t *testing.T, received os.Signal, wantExit int) {
	t.Helper()

	started := make(chan struct{})
	stopped := make(chan struct{})
	stoppedBeforeCancel := make(chan bool, 1)
	deps := testDependencies()
	deps.fetch = func(ctx context.Context, _ string) (codex.Result, error) {
		close(started)
		<-ctx.Done()
		select {
		case <-stopped:
			stoppedBeforeCancel <- true
		default:
			stoppedBeforeCancel <- false
		}
		return codex.Result{}, ctx.Err()
	}

	signals := make(chan os.Signal, 1)
	type result struct {
		exit   int
		stdout string
		stderr string
	}
	finished := make(chan result, 1)
	go func() {
		var stdout, stderr strings.Builder
		exit := runWithSignals(nil, &stdout, &stderr, deps, signals, func() {
			close(stopped)
		})
		finished <- result{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	signals <- received

	var got result
	select {
	case got = <-finished:
	case <-time.After(time.Second):
		t.Fatal("signal did not stop the command")
	}
	if got.exit != wantExit || got.stdout != "" || got.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want silent exit %d", got.exit, got.stdout, got.stderr, wantExit)
	}
	if !<-stoppedBeforeCancel {
		t.Fatal("signal delivery was not stopped before request cancellation")
	}
}
