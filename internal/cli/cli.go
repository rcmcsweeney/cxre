// Package cli coordinates argument parsing, Codex access, and presentation.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rcmcsweeney/cxre/internal/buildinfo"
	"github.com/rcmcsweeney/cxre/internal/codex"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return run(ctx, args, stdout, stderr, defaultDependencies())
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
		problem := publicError(err)
		_ = presentation.RenderError(stderr, problem, parsed.json)
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
	result := reset.Build(reset.Snapshot{
		AvailableCount:  raw.AvailableCount,
		DetailsProvided: raw.DetailsProvided,
		Credits:         credits,
	}, now)

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
		if err := presentation.RenderJSON(stdout, result, presentationOptions); err != nil {
			return renderFailure(stderr, true)
		}
		return 0
	}

	decorateTerminal(stdout, &presentationOptions, deps)
	if err := presentation.RenderHuman(stdout, stderr, result, presentationOptions); err != nil {
		return renderFailure(stderr, false)
	}
	return 0
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

func decorateTerminal(stdout io.Writer, options *presentation.Options, deps dependencies) {
	file, ok := stdout.(*os.File)
	if !ok {
		return
	}
	fd := int(file.Fd())
	if !deps.isTerminal(fd) {
		return
	}

	if width, _, err := deps.termSize(fd); err == nil {
		options.Width = width
	}
	options.Color = supportsColor(deps)
	options.Unicode = supportsUnicode(deps)
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
