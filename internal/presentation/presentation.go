// Package presentation renders reset-credit data for people and machines.
package presentation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rcmcsweeney/cxre/internal/reset"
)

const exactTimeLayout = "Mon 02 Jan 2006 3:04:05 PM MST"

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiCyan   = "\x1b[36m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

// Options control human and JSON presentation without affecting the data.
type Options struct {
	Location *time.Location
	Timezone string
	Now      time.Time
	Color    bool
	Unicode  bool
	Width    int
}

// Error is the stable public representation of an operational failure.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// RenderHuman writes the normal terminal view to out and warnings to warningOut.
func RenderHuman(out, warningOut io.Writer, result reset.Output, options Options) error {
	location := options.Location
	if location == nil {
		location = time.Local
	}

	heading := style("CXRE — Codex Reset Expirations", ansiBold+ansiCyan, options.Color)
	if _, err := fmt.Fprintf(out, "%s\n\n", heading); err != nil {
		return err
	}

	countLine := fmt.Sprintf("Available reset credits: %d", result.AvailableCount)
	if _, err := fmt.Fprintf(out, "%s\n", style(countLine, ansiBold, options.Color)); err != nil {
		return err
	}

	if result.AvailableCount == 0 {
		icon := ""
		if options.Unicode {
			icon = style("✓ ", ansiGreen, options.Color)
		}
		if _, err := fmt.Fprintf(out, "\n%sNo reset credits are currently available.\n", icon); err != nil {
			return err
		}
	} else if len(result.Credits) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if options.Width > 0 && options.Width < 60 {
			if err := renderStacked(out, result.Credits, location, options.Color); err != nil {
				return err
			}
		} else if err := renderTable(out, result.Credits, location, options.Color); err != nil {
			return err
		}
	}

	for _, warning := range result.Warnings {
		prefix := "Warning: "
		if options.Unicode {
			prefix = "! "
		}
		prefix = style(prefix, ansiYellow, options.Color)
		if _, err := fmt.Fprintf(warningOut, "%s%s\n", prefix, warning.Message); err != nil {
			return err
		}
	}

	return nil
}

func renderTable(out io.Writer, credits []reset.Expiration, location *time.Location, color bool) error {
	timestamps := make([]string, len(credits))
	remaining := make([]string, len(credits))
	maxTimestamp := len("Expires")
	maxRemaining := len("Remaining")

	for i, credit := range credits {
		timestamps[i] = exactTime(credit, location)
		remaining[i] = remainingTime(credit)
		maxTimestamp = max(maxTimestamp, len(timestamps[i]))
		maxRemaining = max(maxRemaining, len(remaining[i]))
	}

	if _, err := fmt.Fprintf(out, "%-*s  %s\n", maxTimestamp, "Expires", "Remaining"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, strings.Repeat("-", maxTimestamp+2+maxRemaining)); err != nil {
		return err
	}

	for i, credit := range credits {
		left := fmt.Sprintf("%-*s", maxTimestamp, timestamps[i])
		right := remaining[i]
		code := urgencyColor(credit)
		if code != "" {
			left = style(left, code, color)
			right = style(right, code, color)
		}
		if _, err := fmt.Fprintf(out, "%s  %s\n", left, right); err != nil {
			return err
		}
	}

	return nil
}

func renderStacked(out io.Writer, credits []reset.Expiration, location *time.Location, color bool) error {
	for i, credit := range credits {
		code := urgencyColor(credit)
		expires := exactTime(credit, location)
		remaining := remainingTime(credit)
		if code != "" {
			expires = style(expires, code, color)
			remaining = style(remaining, code, color)
		}
		if _, err := fmt.Fprintf(out, "Expires:   %s\nRemaining: %s\n", expires, remaining); err != nil {
			return err
		}
		if i != len(credits)-1 {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	return nil
}

func exactTime(credit reset.Expiration, location *time.Location) string {
	if credit.DoesNotExpire || credit.ExpiresAt == nil {
		return "Does not expire"
	}
	return credit.ExpiresAt.In(location).Format(exactTimeLayout)
}

func remainingTime(credit reset.Expiration) string {
	if credit.DoesNotExpire || credit.ExpiresAt == nil {
		return "—"
	}
	if credit.Expired {
		return "expired"
	}

	seconds := credit.RemainingSeconds
	switch {
	case seconds >= 24*60*60:
		days := seconds / (24 * 60 * 60)
		hours := (seconds % (24 * 60 * 60)) / (60 * 60)
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	case seconds >= 60*60:
		hours := seconds / (60 * 60)
		minutes := (seconds % (60 * 60)) / 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case seconds >= 60:
		minutes := seconds / 60
		remainingSeconds := seconds % 60
		if remainingSeconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, remainingSeconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func urgencyColor(credit reset.Expiration) string {
	if credit.DoesNotExpire {
		return ""
	}
	if credit.Expired || credit.RemainingSeconds < 60*60 {
		return ansiRed
	}
	if credit.RemainingSeconds < 24*60*60 {
		return ansiYellow
	}
	return ""
}

func style(value, code string, enabled bool) string {
	if !enabled || value == "" {
		return value
	}
	return code + value + ansiReset
}

// RenderJSON writes the stable schema-v1 machine-readable response.
func RenderJSON(out io.Writer, result reset.Output, options Options) error {
	location := options.Location
	if location == nil {
		location = time.Local
	}
	now := options.Now.In(location).Truncate(time.Second)
	timezone := options.Timezone
	if timezone == "" {
		timezone = location.String()
	}

	response := jsonResponse{
		SchemaVersion:  1,
		GeneratedAt:    now.Format(time.RFC3339),
		Timezone:       timezone,
		AvailableCount: result.AvailableCount,
		DetailedCount:  result.DetailedCount,
		MissingCount:   result.MissingCount,
		Complete:       result.Complete,
		Credits:        make([]jsonCredit, 0, len(result.Credits)),
		Warnings:       make([]jsonWarning, 0, len(result.Warnings)),
	}

	for _, credit := range result.Credits {
		item := jsonCredit{
			Expired:       credit.Expired,
			DoesNotExpire: credit.DoesNotExpire,
		}
		if credit.ExpiresAt != nil && !credit.DoesNotExpire {
			expiresAt := credit.ExpiresAt.In(location).Truncate(time.Second).Format(time.RFC3339)
			expiresUnix := credit.ExpiresAt.Unix()
			remainingSeconds := credit.RemainingSeconds
			item.ExpiresAt = &expiresAt
			item.ExpiresAtUnix = &expiresUnix
			item.RemainingSeconds = &remainingSeconds
		}
		response.Credits = append(response.Credits, item)
	}

	for _, warning := range result.Warnings {
		response.Warnings = append(response.Warnings, jsonWarning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}

	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

// RenderError writes a sanitized error in human or JSON form.
func RenderError(out io.Writer, problem Error, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(struct {
			Error Error `json:"error"`
		}{Error: problem})
	}

	if _, err := fmt.Fprintln(out, problem.Message); err != nil {
		return err
	}
	if problem.Action != "" {
		_, err := fmt.Fprintf(out, "\n%s\n", problem.Action)
		return err
	}
	return nil
}

type jsonResponse struct {
	SchemaVersion  int           `json:"schema_version"`
	GeneratedAt    string        `json:"generated_at"`
	Timezone       string        `json:"timezone"`
	AvailableCount int           `json:"available_count"`
	DetailedCount  int           `json:"detailed_count"`
	MissingCount   int           `json:"missing_count"`
	Complete       bool          `json:"complete"`
	Credits        []jsonCredit  `json:"credits"`
	Warnings       []jsonWarning `json:"warnings"`
}

type jsonCredit struct {
	ExpiresAt        *string `json:"expires_at"`
	ExpiresAtUnix    *int64  `json:"expires_at_unix"`
	RemainingSeconds *int64  `json:"remaining_seconds"`
	Expired          bool    `json:"expired"`
	DoesNotExpire    bool    `json:"does_not_expire"`
}

type jsonWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
