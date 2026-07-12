package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout    = 10 * time.Second
	shutdownGrace     = 100 * time.Millisecond
	maxMessageBytes   = 1024 * 1024
	maxCapturedStderr = 32 * 1024
	fiveHourMinMins   = 285
	fiveHourMaxMins   = 315
	weeklyMinMins     = 9576
	weeklyMaxMins     = 10584
)

// Result contains the reset-credit contract and the recognized Codex usage
// windows from account/rateLimits/read. AvailableCount is authoritative.
// DetailsProvided distinguishes a null/missing detail list from a fetched
// (possibly empty or capped) list.
type Result struct {
	AvailableCount  int
	DetailsProvided bool
	Credits         []Credit
	FiveHour        *UsageWindow
	Weekly          *UsageWindow
}

// UsageWindow is a recognized Codex quota window from account/rateLimits/read.
// A nil pointer on Result means Codex did not provide that window. ResetsAt is
// nil when the server omits it and is otherwise normalized to UTC.
type UsageWindow struct {
	UsedPercent float64
	ResetsAt    *time.Time
}

type usageWindowKind uint8

const (
	usageWindowUnknown usageWindowKind = iota
	usageWindowFiveHour
	usageWindowWeekly
)

// Credit is one app-server detail row. IDs are opaque and must not be printed
// by presentation code. The Provided fields distinguish an explicit null from
// a field absent on older or otherwise incomplete app-server responses.
type Credit struct {
	ID                string
	ResetType         string
	Status            string
	GrantedAt         *time.Time
	GrantedAtProvided bool
	ExpiresAt         *time.Time
	ExpiresAtProvided bool
}

// Fetch obtains reset-credit data through one local `codex app-server --stdio`
// process. It does not read credential files or keychains and never refreshes a
// token explicitly; credential ownership stays with Codex.
func Fetch(ctx context.Context, clientVersion string) (Result, error) {
	executable, err := resolveExecutable()
	if err != nil {
		return Result{}, err
	}
	return fetch(ctx, fetchOptions{
		executable:         executable.path,
		explicitExecutable: executable.explicit,
		clientVersion:      clientVersion,
		timeout:            defaultTimeout,
	})
}

type fetchOptions struct {
	executable         string
	explicitExecutable bool
	clientVersion      string
	timeout            time.Duration
}

func fetch(parent context.Context, options fetchOptions) (result Result, resultErr error) {
	timeout := options.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, options.executable, "app-server", "--stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, failure(CodeProtocol, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return Result{}, failure(CodeProtocol, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return Result{}, failure(CodeProtocol, err)
	}

	var captured limitedCapture
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&captured, stderr)
		close(stderrDone)
	}()

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		<-stderrDone
		if ctx.Err() != nil {
			return Result{}, failure(CodeTimeout, ctx.Err())
		}
		if options.explicitExecutable {
			return Result{}, failure(CodeCodexInvalidExecutable, err)
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return Result{}, failure(CodeCodexNotFound, err)
		}
		return Result{}, failure(CodeProtocol, err)
	}
	defer func() {
		shutdownProcess(ctx, cmd, stdin, stderrDone)
		var transportFailure *transportIOError
		if errors.As(resultErr, &transportFailure) {
			resultErr = classifyTransportFailure(transportFailure, captured.String())
		}
	}()

	transport := jsonlTransport{
		ctx:     ctx,
		encoder: json.NewEncoder(stdin),
		scanner: bufio.NewScanner(stdout),
	}
	transport.scanner.Buffer(make([]byte, 4096), maxMessageBytes)

	version := options.clientVersion
	if version == "" {
		version = "dev"
	}

	if err := transport.request(0, "initialize", initializeParams{
		ClientInfo: clientInfo{Name: "cxre", Title: "CXRE", Version: version},
	}, nil); err != nil {
		return Result{}, err
	}
	if err := transport.notify("initialized", struct{}{}); err != nil {
		return Result{}, err
	}

	var account accountReadResult
	if err := transport.request(1, "account/read", accountReadParams{RefreshToken: false}, &account); err != nil {
		return Result{}, err
	}
	if err := validateAccount(account); err != nil {
		return Result{}, err
	}

	var rateLimits json.RawMessage
	if err := transport.request(2, "account/rateLimits/read", nil, &rateLimits); err != nil {
		return Result{}, err
	}
	result, err = parseRateLimits(rateLimits)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// shutdownProcess asks app-server to exit cleanly by closing its input. Wait is
// called exactly once. A canceled request is terminated immediately; otherwise
// the process receives a short grace period before a kill fallback. The stderr
// drain is joined after reaping so no child output can leak or leave a goroutine
// behind.
func shutdownProcess(
	ctx context.Context,
	cmd *exec.Cmd,
	stdin io.Closer,
	stderrDone <-chan struct{},
) {
	_ = stdin.Close()

	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	if ctx.Err() != nil {
		_ = cmd.Process.Kill()
		<-waitDone
		<-stderrDone
		return
	}

	timer := time.NewTimer(shutdownGrace)
	select {
	case <-waitDone:
		if !timer.Stop() {
			<-timer.C
		}
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-waitDone
	}
	<-stderrDone
}

type resolvedExecutable struct {
	path     string
	explicit bool
}

func resolveExecutable() (resolvedExecutable, error) {
	return resolvePath(os.LookupEnv, exec.LookPath)
}

func resolvePath(
	lookupEnv func(string) (string, bool),
	lookPath func(string) (string, error),
) (resolvedExecutable, error) {
	if configured, ok := lookupEnv("CXRE_CODEX"); ok {
		if configured == "" {
			return resolvedExecutable{}, failure(CodeCodexInvalidExecutable, exec.ErrNotFound)
		}
		path, err := lookPath(configured)
		if err != nil {
			return resolvedExecutable{}, failure(CodeCodexInvalidExecutable, err)
		}
		return resolvedExecutable{path: path, explicit: true}, nil
	}

	path, err := lookPath("codex")
	if err != nil {
		return resolvedExecutable{}, failure(CodeCodexNotFound, err)
	}
	return resolvedExecutable{path: path}, nil
}

type initializeParams struct {
	ClientInfo clientInfo `json:"clientInfo"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type accountReadParams struct {
	RefreshToken bool `json:"refreshToken"`
}

type accountReadResult struct {
	Account *accountInfo `json:"account"`
}

// accountInfo intentionally names only the discriminator CXRE uses. The JSON
// decoder discards email, plan, and any future account metadata immediately.
type accountInfo struct {
	Type string `json:"type"`
}

func validateAccount(result accountReadResult) error {
	if result.Account == nil {
		return failure(CodeAuthMissing, nil)
	}
	if result.Account.Type == "" {
		return failure(CodeProtocol, nil)
	}
	if result.Account.Type != "chatgpt" {
		return failure(CodeUnsupportedAuth, nil)
	}
	return nil
}

type rawResetCredits struct {
	AvailableCount json.RawMessage `json:"availableCount"`
	Credits        json.RawMessage `json:"credits"`
}

type rawCredit struct {
	ID        string          `json:"id"`
	ResetType string          `json:"resetType"`
	Status    string          `json:"status"`
	GrantedAt json.RawMessage `json:"grantedAt"`
	ExpiresAt json.RawMessage `json:"expiresAt"`
}

func parseRateLimits(raw json.RawMessage) (Result, error) {
	if isNullOrMissing(raw) {
		return Result{}, failure(CodeProtocol, nil)
	}
	var envelope struct {
		ResetCredits        json.RawMessage `json:"rateLimitResetCredits"`
		RateLimits          json.RawMessage `json:"rateLimits"`
		RateLimitsByLimitID json.RawMessage `json:"rateLimitsByLimitId"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Result{}, failure(CodeProtocol, err)
	}
	if len(bytes.TrimSpace(envelope.ResetCredits)) == 0 {
		return Result{}, failure(CodeCodexTooOld, nil)
	}
	if bytes.Equal(bytes.TrimSpace(envelope.ResetCredits), []byte("null")) {
		return Result{}, failure(CodeResetCreditsUnavailable, nil)
	}

	var reset rawResetCredits
	if err := json.Unmarshal(envelope.ResetCredits, &reset); err != nil {
		return Result{}, failure(CodeProtocol, err)
	}
	if isNullOrMissing(reset.AvailableCount) {
		return Result{}, failure(CodeProtocol, nil)
	}
	var available int
	if err := json.Unmarshal(reset.AvailableCount, &available); err != nil || available < 0 {
		return Result{}, failure(CodeProtocol, err)
	}

	result := Result{AvailableCount: available}
	parseUsageLimits(&result, envelope.RateLimits, envelope.RateLimitsByLimitID)
	if isNullOrMissing(reset.Credits) {
		return result, nil
	}

	var rows []rawCredit
	if err := json.Unmarshal(reset.Credits, &rows); err != nil || rows == nil {
		return Result{}, failure(CodeProtocol, err)
	}
	result.DetailsProvided = true
	result.Credits = make([]Credit, 0, len(rows))
	for _, row := range rows {
		grantedProvided, grantedAt, err := parseUnixTime(row.GrantedAt)
		if err != nil {
			return Result{}, err
		}
		expiresProvided, expiresAt, err := parseUnixTime(row.ExpiresAt)
		if err != nil {
			return Result{}, err
		}
		result.Credits = append(result.Credits, Credit{
			ID:                row.ID,
			ResetType:         row.ResetType,
			Status:            row.Status,
			GrantedAt:         grantedAt,
			GrantedAtProvided: grantedProvided,
			ExpiresAt:         expiresAt,
			ExpiresAtProvided: expiresProvided,
		})
	}
	return result, nil
}

// parseUsageLimits extracts only the standard five-hour and weekly Codex
// windows. Quota summaries are ancillary to CXRE's reset-credit contract, so
// missing, malformed, or unfamiliar window data is ignored instead of turning
// an otherwise valid reset-credit response into an operational failure.
//
// The backward-compatible rateLimits bucket is authoritative when present.
// The codex entry in rateLimitsByLimitId fills any window or reset timestamp
// that the legacy bucket did not provide; unrelated metered buckets are
// deliberately ignored. A fallback timestamp never replaces the legacy
// percentage.
func parseUsageLimits(result *Result, legacy, byLimitID json.RawMessage) {
	collectUsageWindows(result, legacy)
	if usageWindowsComplete(result) {
		return
	}

	var buckets map[string]json.RawMessage
	if isNullOrMissing(byLimitID) || json.Unmarshal(byLimitID, &buckets) != nil {
		return
	}
	collectUsageWindows(result, buckets["codex"])
}

func usageWindowsComplete(result *Result) bool {
	return result.FiveHour != nil && result.FiveHour.ResetsAt != nil &&
		result.Weekly != nil && result.Weekly.ResetsAt != nil
}

func collectUsageWindows(result *Result, raw json.RawMessage) {
	if isNullOrMissing(raw) {
		return
	}
	var bucket struct {
		Primary   json.RawMessage `json:"primary"`
		Secondary json.RawMessage `json:"secondary"`
	}
	if json.Unmarshal(raw, &bucket) != nil {
		return
	}
	for _, candidate := range []json.RawMessage{bucket.Primary, bucket.Secondary} {
		window, kind, ok := parseUsageWindow(candidate)
		if !ok {
			continue
		}
		switch kind {
		case usageWindowFiveHour:
			mergeUsageWindow(&result.FiveHour, window)
		case usageWindowWeekly:
			mergeUsageWindow(&result.Weekly, window)
		}
	}
}

func mergeUsageWindow(existing **UsageWindow, candidate *UsageWindow) {
	if *existing == nil {
		*existing = candidate
		return
	}
	if (*existing).ResetsAt == nil && candidate.ResetsAt != nil {
		reset := *candidate.ResetsAt
		(*existing).ResetsAt = &reset
	}
}

func parseUsageWindow(raw json.RawMessage) (*UsageWindow, usageWindowKind, bool) {
	if isNullOrMissing(raw) {
		return nil, usageWindowUnknown, false
	}
	var window struct {
		UsedPercent       json.RawMessage `json:"usedPercent"`
		WindowDurationMin json.RawMessage `json:"windowDurationMins"`
		ResetsAt          json.RawMessage `json:"resetsAt"`
	}
	if json.Unmarshal(raw, &window) != nil ||
		isNullOrMissing(window.UsedPercent) ||
		isNullOrMissing(window.WindowDurationMin) {
		return nil, usageWindowUnknown, false
	}

	var usedPercent float64
	var duration int64
	if json.Unmarshal(window.UsedPercent, &usedPercent) != nil ||
		json.Unmarshal(window.WindowDurationMin, &duration) != nil {
		return nil, usageWindowUnknown, false
	}
	kind := classifyUsageWindow(duration)
	if kind == usageWindowUnknown {
		return nil, usageWindowUnknown, false
	}

	var resetsAt *time.Time
	if !isNullOrMissing(window.ResetsAt) {
		var seconds int64
		if json.Unmarshal(window.ResetsAt, &seconds) == nil {
			parsed := time.Unix(seconds, 0).UTC()
			resetsAt = &parsed
		}
	}
	return &UsageWindow{
		UsedPercent: usedPercent,
		ResetsAt:    resetsAt,
	}, kind, true
}

func classifyUsageWindow(durationMins int64) usageWindowKind {
	switch {
	case durationMins >= fiveHourMinMins && durationMins <= fiveHourMaxMins:
		return usageWindowFiveHour
	case durationMins >= weeklyMinMins && durationMins <= weeklyMaxMins:
		return usageWindowWeekly
	default:
		return usageWindowUnknown
	}
}

func parseUnixTime(raw json.RawMessage) (provided bool, value *time.Time, err error) {
	if len(raw) == 0 {
		return false, nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true, nil, nil
	}
	var seconds int64
	if unmarshalErr := json.Unmarshal(raw, &seconds); unmarshalErr != nil {
		return false, nil, failure(CodeProtocol, unmarshalErr)
	}
	parsed := time.Unix(seconds, 0).UTC()
	return true, &parsed, nil
}

func isNullOrMissing(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

type jsonlTransport struct {
	ctx     context.Context
	encoder *json.Encoder
	scanner *bufio.Scanner
}

type wireRequest struct {
	Method string `json:"method"`
	ID     *int64 `json:"id,omitempty"`
	Params any    `json:"params,omitempty"`
}

type wireResponse struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (t *jsonlTransport) request(id int64, method string, params any, target any) error {
	if err := t.encoder.Encode(wireRequest{Method: method, ID: &id, Params: params}); err != nil {
		return t.ioFailure(err)
	}
	response, err := t.readResponse(id)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	if isNullOrMissing(response) {
		return failure(CodeProtocol, nil)
	}
	if err := json.Unmarshal(response, target); err != nil {
		return failure(CodeProtocol, err)
	}
	return nil
}

func (t *jsonlTransport) notify(method string, params any) error {
	if err := t.encoder.Encode(wireRequest{Method: method, Params: params}); err != nil {
		return t.ioFailure(err)
	}
	return nil
}

func (t *jsonlTransport) readResponse(expectedID int64) (json.RawMessage, error) {
	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var message wireResponse
		if err := json.Unmarshal(line, &message); err != nil {
			return nil, failure(CodeProtocol, err)
		}
		// Notifications have no id and may arrive between any two responses.
		if len(message.ID) == 0 {
			if message.Method == "" {
				return nil, failure(CodeProtocol, nil)
			}
			continue
		}
		// Server-initiated requests are not expected because CXRE opts into no
		// capabilities. Failing closed avoids hanging on a request we cannot
		// safely answer.
		if message.Method != "" {
			return nil, failure(CodeProtocol, nil)
		}
		var id int64
		if err := json.Unmarshal(message.ID, &id); err != nil || id != expectedID {
			return nil, failure(CodeProtocol, err)
		}
		if message.Error != nil {
			return nil, classifyRPCError(message.Error)
		}
		if isNullOrMissing(message.Result) {
			return nil, failure(CodeProtocol, nil)
		}
		return message.Result, nil
	}
	return nil, t.ioFailure(t.scanner.Err())
}

// transportIOError is kept private until fetch has reaped the child and joined
// the stderr drain. It deliberately retains no raw protocol or process text.
type transportIOError struct {
	timedOut bool
}

func (*transportIOError) Error() string { return "Codex transport failed" }

func (t *jsonlTransport) ioFailure(_ error) error {
	return &transportIOError{timedOut: t.ctx.Err() != nil}
}

func classifyTransportFailure(problem *transportIOError, text string) error {
	if problem.timedOut {
		return failure(CodeTimeout, nil)
	}
	if looksLikeTooOldFailure(text) {
		return failure(CodeCodexTooOld, nil)
	}
	if looksLikeTimeout(text) {
		return failure(CodeTimeout, nil)
	}
	if looksLikeNetworkFailure(text) {
		return failure(CodeNetwork, nil)
	}
	return failure(CodeProtocol, nil)
}

func classifyRPCError(rpc *rpcError) error {
	message := strings.ToLower(rpc.Message)
	if rpc.Code == -32601 || containsAny(message,
		"method not found", "unknown method", "unsupported method") {
		return failure(CodeCodexTooOld, nil)
	}
	if containsAny(message,
		"unauthorized", "authentication", "not logged in", "login required",
		"missing auth", "sign in") {
		return failure(CodeAuthMissing, nil)
	}
	if looksLikeTimeout(message) {
		return failure(CodeTimeout, nil)
	}
	if looksLikeNetworkFailure(message) {
		return failure(CodeNetwork, nil)
	}
	return failure(CodeProtocol, nil)
}

func looksLikeNetworkFailure(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower,
		"network", "connection refused", "connection reset", "dns",
		"offline", "could not resolve", "name resolution", "no such host",
		"failed to connect", "error sending request", "service unavailable",
		"bad gateway", "gateway timeout", "http 502", "http 503", "http 504")
}

func looksLikeTimeout(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower, "timed out", "timeout", "deadline exceeded")
}

func looksLikeTooOldFailure(text string) bool {
	lower := strings.ToLower(text)
	missingAppServer := containsAny(lower,
		"unrecognized subcommand 'app-server'", "unknown command app-server", "no such command app-server")
	missingStdio := strings.Contains(lower, "--stdio") && containsAny(lower,
		"unexpected argument", "unknown argument", "unrecognized option", "wasn't expected")
	return missingAppServer || missingStdio
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

// limitedCapture drains stderr without allowing unbounded memory use. Its
// contents are used only for coarse failure classification and are never
// returned, logged, or included in an Error.
type limitedCapture struct {
	mu  sync.Mutex
	buf []byte
}

func (c *limitedCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining := maxCapturedStderr - len(c.buf)
	if remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		c.buf = append(c.buf, p[:remaining]...)
	}
	return len(p), nil
}

func (c *limitedCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf)
}
