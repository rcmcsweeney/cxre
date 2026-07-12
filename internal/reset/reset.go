// Package reset normalizes Codex reset-credit snapshots into presentation-ready
// expiration data. It deliberately keeps opaque credit IDs internal to the
// process; renderers must never expose them.
package reset

import (
	"fmt"
	"sort"
	"time"
)

const (
	// WarningCodePartialDetails identifies a successful response for which Codex
	// supplied fewer usable detail rows than its authoritative available count.
	WarningCodePartialDetails = "partial_reset_credit_details"

	// WarningCodeInconsistentDetails identifies detail rows that contradict the
	// authoritative count or use a status this version of CXRE cannot classify.
	WarningCodeInconsistentDetails = "inconsistent_reset_credit_details"
)

const (
	statusAvailable = "available"
	statusRedeeming = "redeeming"
	statusRedeemed  = "redeemed"
	statusUnknown   = "unknown"
)

// Credit is a transport-neutral reset-credit detail row. ID remains available
// for internal correlation but is explicitly excluded from JSON serialization.
// A nil ExpiresAt on an available detail means that the credit does not expire.
type Credit struct {
	ID        string     `json:"-"`
	Status    string     `json:"-"`
	ExpiresAt *time.Time `json:"-"`
}

// Snapshot is the reset-credit portion of an account/rateLimits/read response.
// AvailableCount is authoritative. DetailsProvided distinguishes a null detail
// list from a fetched list that happens to be empty.
type Snapshot struct {
	AvailableCount  int
	DetailsProvided bool
	Credits         []Credit
}

// Expiration is one available credit prepared for terminal or JSON rendering.
// ID is retained only for in-process correlation and must never be displayed.
type Expiration struct {
	ID               string     `json:"-"`
	ExpiresAt        *time.Time `json:"-"`
	RemainingSeconds int64      `json:"-"`
	Expired          bool       `json:"-"`
	DoesNotExpire    bool       `json:"-"`
}

// Warning is a stable machine-readable warning plus safe user-facing prose.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Output is the normalized reset-credit view consumed by presentation code.
// Credits is always sorted by soonest expiration, with non-expiring credits
// last and opaque ID, when present, as the deterministic tie-break.
// AvailableCount always reflects the authoritative server count.
type Output struct {
	AvailableCount int
	DetailedCount  int
	MissingCount   int
	Complete       bool
	Credits        []Expiration
	Warnings       []Warning
}

// Build normalizes snapshot at the supplied instant. It never mutates snapshot
// or aliases its time pointers. Known non-available rows are ignored; unknown
// statuses make the result incomplete so a new server status cannot silently be
// mistaken for an available credit.
func Build(snapshot Snapshot, now time.Time) Output {
	availableCount := snapshot.AvailableCount
	invalidCount := availableCount < 0
	if invalidCount {
		availableCount = 0
	}

	output := Output{
		AvailableCount: availableCount,
		Credits:        make([]Expiration, 0),
		Warnings:       make([]Warning, 0),
	}

	// A null detail list is intentionally different from an empty fetched list.
	// Ignore any contradictory in-memory rows when DetailsProvided is false.
	if !snapshot.DetailsProvided {
		output.MissingCount = availableCount
		output.Complete = availableCount == 0 && !invalidCount && len(snapshot.Credits) == 0
		if output.MissingCount > 0 {
			output.Warnings = append(output.Warnings, partialWarning(output.MissingCount))
		}
		if invalidCount || len(snapshot.Credits) > 0 {
			output.Warnings = append(output.Warnings, inconsistentWarning())
		}
		return output
	}

	unknownStatus := false
	inconsistentIdentity := false
	seenIDs := make(map[string]struct{}, len(snapshot.Credits))
	selectedAvailableIDs := make(map[string]struct{}, len(snapshot.Credits))
	for _, credit := range snapshot.Credits {
		if credit.ID == "" {
			// Every fetched detail row is expected to have an identity, including
			// rows in non-available states. Without one, the snapshot cannot be
			// correlated reliably even when no expiration would be displayed.
			inconsistentIdentity = true
		} else {
			if _, seen := seenIDs[credit.ID]; seen {
				// Even identical rows are inconsistent: a credit identity is expected
				// to occur at most once in one fetched snapshot.
				inconsistentIdentity = true
			} else {
				seenIDs[credit.ID] = struct{}{}
			}
		}

		switch credit.Status {
		case statusAvailable:
			if credit.ID == "" {
				// Preserve usable data, but do not merge unrelated anonymous rows or
				// pretend that they can be correlated reliably.
				output.Credits = append(output.Credits, expirationFrom(credit, now))
				continue
			}
			if _, selected := selectedAvailableIDs[credit.ID]; selected {
				// Retain the first available row within this ambiguous identity
				// group. A repeated row was already marked inconsistent.
				continue
			}
			selectedAvailableIDs[credit.ID] = struct{}{}
			output.Credits = append(output.Credits, expirationFrom(credit, now))
		case statusRedeeming, statusRedeemed:
			// These are known states, but they are not available credits.
		case statusUnknown:
			unknownStatus = true
		default:
			unknownStatus = true
		}
	}

	sortExpirations(output.Credits)

	overCount := len(output.Credits) > availableCount
	if overCount {
		// The count is authoritative. Retain the soonest expirations rather than
		// presenting more available credits than Codex says exist.
		output.Credits = output.Credits[:availableCount]
	}

	output.DetailedCount = len(output.Credits)
	output.MissingCount = availableCount - output.DetailedCount
	output.Complete = !invalidCount && !unknownStatus && !inconsistentIdentity && !overCount && output.MissingCount == 0

	if output.MissingCount > 0 {
		output.Warnings = append(output.Warnings, partialWarning(output.MissingCount))
	}
	if invalidCount || unknownStatus || inconsistentIdentity || overCount {
		output.Warnings = append(output.Warnings, inconsistentWarning())
	}

	return output
}

func expirationFrom(credit Credit, now time.Time) Expiration {
	expiration := Expiration{ID: credit.ID}
	if credit.ExpiresAt == nil {
		expiration.DoesNotExpire = true
		return expiration
	}

	// Copy the value so callers cannot change normalized output by mutating an
	// input pointer after Build returns.
	expiresAt := *credit.ExpiresAt
	expiration.ExpiresAt = &expiresAt
	if !expiresAt.After(now) {
		expiration.Expired = true
		return expiration
	}

	expiration.RemainingSeconds = int64(expiresAt.Sub(now) / time.Second)
	return expiration
}

func sortExpirations(credits []Expiration) {
	sort.SliceStable(credits, func(i, j int) bool {
		left, right := credits[i], credits[j]
		if left.DoesNotExpire || left.ExpiresAt == nil {
			if right.DoesNotExpire || right.ExpiresAt == nil {
				return left.ID < right.ID
			}
			return false
		}
		if right.DoesNotExpire || right.ExpiresAt == nil {
			return true
		}
		if left.ExpiresAt.Equal(*right.ExpiresAt) {
			return left.ID < right.ID
		}
		return left.ExpiresAt.Before(*right.ExpiresAt)
	})
}

func partialWarning(missing int) Warning {
	credit := "credits"
	if missing == 1 {
		credit = "credit"
	}
	return Warning{
		Code:    WarningCodePartialDetails,
		Message: fmt.Sprintf("Expiration details are unavailable for %d reset %s.", missing, credit),
	}
}

func inconsistentWarning() Warning {
	return Warning{
		Code:    WarningCodeInconsistentDetails,
		Message: "Some reset-credit details could not be classified reliably.",
	}
}
