// Package cli coordinates argument parsing, Codex access, and presentation.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rcmcsweeney/cxre/internal/buildinfo"
	"github.com/rcmcsweeney/cxre/internal/codex"
	"github.com/rcmcsweeney/cxre/internal/limits"
	"github.com/rcmcsweeney/cxre/internal/presentation"
	"github.com/rcmcsweeney/cxre/internal/reset"
	"golang.org/x/term"
)

const requestTimeout = 10 * time.Second

type fetchFunc func(context.Context, string) (codex.Result, error)

type commandHandler func(context.Context, options, io.Writer, io.Writer, dependencies) int

var commandRegistry = map[string]commandHandler{
	"": runExpirations,
}

type dependencies struct {
	fetch      fetchFunc
	now        func() time.Time
	lookupEnv  func(string) (string, bool)
	isTerminal func(int) bool
	termSize   func(int) (int, int, error)
	readlink   func(string) (string, error)
	local      *time.Location
	goos       string
}

func defaultDependencies() dependencies {
	return dependencies{
		fetch:      codex.Fetch,
		now:        time.Now,
		lookupEnv:  os.LookupEnv,
		isTerminal: term.IsTerminal,
		termSize:   term.GetSize,
		readlink:   os.Readlink,
		local:      time.Local,
		goos:       runtime.GOOS,
	}
}

// Run executes CXRE and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, handledSignals()...)
	return runWithSignals(args, stdout, stderr, defaultDependencies(), signals, func() {
		signal.Stop(signals)
	})
}

// runWithSignals keeps process-signal mechanics outside the command logic so
// cancellation behavior is deterministic and testable. The first signal is
// unregistered before the request is canceled; a second signal therefore uses
// the operating system's default behavior instead of waiting for cleanup.
func runWithSignals(
	args []string,
	stdout, stderr io.Writer,
	deps dependencies,
	signals <-chan os.Signal,
	stopSignals func(),
) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stagedStdout := &stagedOutput{destination: stdout}
	stagedStderr := &stagedOutput{destination: stderr}

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(stopSignals)
	}

	done := make(chan struct{})
	signalExit := make(chan int, 1)
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		handle := func(received os.Signal) {
			exitCode := exitCodeForSignal(received)
			stop()
			signalExit <- exitCode
			cancel()
		}
		select {
		case received := <-signals:
			handle(received)
		case <-done:
			// If completion and a signal raced, prefer the signal that was already
			// delivered rather than returning a successful or operational status.
			select {
			case received := <-signals:
				handle(received)
			default:
			}
		}
	}()

	exitCode := run(ctx, args, stagedStdout, stagedStderr, deps)
	close(done)
	watcher.Wait()
	stop()

	select {
	case interruptedExit := <-signalExit:
		return interruptedExit
	default:
	}
	// A signal can be queued after the watcher observes normal completion but
	// before signal.Stop returns. Drain that final delivery before committing
	// staged output so the interruption remains silent.
	select {
	case received := <-signals:
		return exitCodeForSignal(received)
	default:
	}

	if err := stagedStdout.commit(); err != nil {
		parsed, _ := parse(args)
		return renderFailure(stderr, parsed.json)
	}
	if err := stagedStderr.commit(); err != nil {
		// stderr itself is unavailable, so there is nowhere safe to report the
		// otherwise sanitized output error.
		return 1
	}
	return exitCode
}

// stagedOutput prevents a late signal from leaving a successful document or a
// warning behind while runWithSignals returns an interruption status. Terminal
// detection still sees the destination stream through streamFD below.
type stagedOutput struct {
	destination io.Writer
	bytes.Buffer
}

func (output *stagedOutput) commit() error {
	_, err := io.Copy(output.destination, &output.Buffer)
	return err
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	parsed, err := parse(args)
	if err != nil {
		return renderUsageError(stderr, parsed.json, err)
	}

	handler, ok := commandRegistry[parsed.command]
	if !ok {
		return renderUsageError(stderr, parsed.json, fmt.Errorf("unknown command %q", parsed.command))
	}

	if parsed.help {
		if err := writeHelp(stdout); err != nil {
			return renderFailure(stderr, parsed.json)
		}
		return 0
	}
	if parsed.version {
		if _, err := fmt.Fprintln(stdout, buildinfo.String()); err != nil {
			return renderFailure(stderr, parsed.json)
		}
		return 0
	}

	return handler(ctx, parsed, stdout, stderr, deps)
}

func runExpirations(ctx context.Context, parsed options, stdout, stderr io.Writer, deps dependencies) int {

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	raw, err := deps.fetch(requestCtx, buildinfo.Current().Version)
	if err != nil {
		if ctx.Err() != nil {
			return 1
		}
		problem := publicError(err)
		_ = presentation.RenderError(stderr, problem, parsed.json)
		return 1
	}
	if ctx.Err() != nil {
		return 1
	}

	now := deps.now()
	credits := make([]reset.Credit, 0, len(raw.Credits))
	for _, credit := range raw.Credits {
		status := credit.Status
		// An explicit null expiresAt means a non-expiring credit. A missing
		// expiresAt field on an available row instead means this client cannot
		// classify the row, so preserve that evidence as an unknown status.
		if !credit.ExpiresAtProvided && status == "available" {
			status = "unknown"
		}
		credits = append(credits, reset.Credit{
			ID:        credit.ID,
			Status:    status,
			ExpiresAt: credit.ExpiresAt,
		})
	}
	resetResult := reset.Build(reset.Snapshot{
		AvailableCount:  raw.AvailableCount,
		DetailsProvided: raw.DetailsProvided,
		Credits:         credits,
	}, now)
	report := presentation.Report{
		Limits: limits.Build(limits.Snapshot{
			FiveHour: limitSnapshot(raw.FiveHour),
			Weekly:   limitSnapshot(raw.Weekly),
		}, now),
		Resets: resetResult,
	}

	location := deps.local
	if location == nil {
		location = time.Local
	}
	if parsed.utc {
		location = time.UTC
	}

	presentationOptions := presentation.Options{
		Location: location,
		Timezone: timezoneName(location, now, deps),
		Now:      now,
	}
	if parsed.json {
		if err := presentation.RenderJSON(stdout, report, presentationOptions); err != nil {
			return renderFailure(stderr, true)
		}
		return 0
	}

	decorateTerminal(stdout, stderr, &presentationOptions, deps)
	if err := presentation.RenderHuman(stdout, stderr, report, presentationOptions); err != nil {
		return renderFailure(stderr, false)
	}
	return 0
}

func limitSnapshot(window *codex.UsageWindow) *limits.WindowSnapshot {
	if window == nil {
		return nil
	}
	return &limits.WindowSnapshot{
		UsedPercent: window.UsedPercent,
		ResetsAt:    window.ResetsAt,
	}
}

func renderUsageError(stderr io.Writer, asJSON bool, err error) int {
	if asJSON {
		_ = presentation.RenderError(stderr, presentation.Error{
			Code:    "usage",
			Message: err.Error(),
			Action:  "Run `cxre --help` for usage.",
		}, true)
		return 2
	}
	_, _ = fmt.Fprintf(stderr, "cxre: %s\n\n", err)
	_ = writeHelp(stderr)
	return 2
}

func publicError(err error) presentation.Error {
	var codexError *codex.Error
	if errors.As(err, &codexError) {
		return presentation.Error{
			Code:    string(codexError.Code),
			Message: codexError.Message,
			Action:  codexError.Action,
		}
	}
	return presentation.Error{
		Code:    "protocol",
		Message: "Unable to read reset-credit expirations from Codex.",
		Action:  "Please try again. If the problem continues, update the Codex CLI.",
	}
}

func renderFailure(stderr io.Writer, asJSON bool) int {
	_ = presentation.RenderError(stderr, presentation.Error{
		Code:    "output",
		Message: "Unable to write CXRE output.",
	}, asJSON)
	return 1
}

func decorateTerminal(stdout, stderr io.Writer, options *presentation.Options, deps dependencies) {
	options.WarningColor = terminalSupportsColor(stderr, deps)

	fd, ok := streamFD(stdout)
	if !ok {
		return
	}
	if !deps.isTerminal(fd) {
		return
	}

	if width, _, err := deps.termSize(fd); err == nil {
		options.Width = width
	}
	options.Color = supportsColor(deps)
	options.Unicode = supportsUnicode(deps)
}

func terminalSupportsColor(out io.Writer, deps dependencies) bool {
	fd, ok := streamFD(out)
	if !ok || !deps.isTerminal(fd) {
		return false
	}
	return supportsColor(deps)
}

func streamFD(out io.Writer) (int, bool) {
	if staged, ok := out.(*stagedOutput); ok {
		out = staged.destination
	}
	file, ok := out.(*os.File)
	if !ok {
		return 0, false
	}
	return int(file.Fd()), true
}

func supportsColor(deps dependencies) bool {
	if _, found := deps.lookupEnv("NO_COLOR"); found {
		return false
	}
	if value, _ := deps.lookupEnv("TERM"); strings.EqualFold(value, "dumb") {
		return false
	}
	if deps.goos != "windows" {
		return true
	}
	return windowsTerminal(deps)
}

func supportsUnicode(deps dependencies) bool {
	if value, _ := deps.lookupEnv("TERM"); strings.EqualFold(value, "dumb") {
		return false
	}
	if deps.goos == "windows" {
		return windowsTerminal(deps)
	}

	locale := ""
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value, found := deps.lookupEnv(key); found && value != "" {
			locale = value
			break
		}
	}
	if locale == "" {
		return true
	}
	locale = strings.ToLower(locale)
	return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
}

func windowsTerminal(deps dependencies) bool {
	for _, key := range []string{"WT_SESSION", "ANSICON"} {
		if value, found := deps.lookupEnv(key); found && value != "" {
			return true
		}
	}
	if value, found := deps.lookupEnv("ConEmuANSI"); found && strings.EqualFold(value, "ON") {
		return true
	}
	if value, found := deps.lookupEnv("TERM"); found {
		value = strings.ToLower(value)
		return strings.Contains(value, "xterm") || strings.Contains(value, "vt100") || strings.Contains(value, "cygwin")
	}
	return false
}

func timezoneName(location *time.Location, now time.Time, deps dependencies) string {
	if location == nil {
		location = time.Local
	}
	if location == time.UTC || location.String() == "UTC" {
		return "UTC"
	}
	if name := location.String(); name != "" && name != "Local" {
		return name
	}

	if value, found := deps.lookupEnv("TZ"); found {
		if name := zoneinfoName(value); name != "" {
			return name
		}
	}
	if deps.readlink != nil {
		if target, err := deps.readlink("/etc/localtime"); err == nil {
			if name := zoneinfoName(target); name != "" {
				return name
			}
		}
	}

	name, _ := now.In(location).Zone()
	if name != "" {
		return name
	}
	return "Local"
}

func zoneinfoName(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if index := strings.LastIndex(value, "/zoneinfo/"); index >= 0 {
		value = value[index+len("/zoneinfo/"):]
	}
	value = strings.TrimPrefix(value, "posix/")
	value = strings.TrimPrefix(value, "right/")
	if value == "UTC" || value == "Etc/UTC" {
		return value
	}
	if !strings.Contains(value, "/") || strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
		return ""
	}
	return value
}
