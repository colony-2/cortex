package cortex

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

// Discovery reads forms, so it must never become part of compact polling.
type occurrenceReader interface {
	ListPendingInputsPage(context.Context, string, input.PendingInputOptions) (input.PendingInputPage, error)
}
type timingEntry struct {
	version    string
	occurrence input.PendingInputOccurrence
	retry      time.Time
}
type tenantTiming struct {
	gate    chan struct{}
	entries map[string]timingEntry
	users   int
	used    time.Time
}
type pendingTiming struct {
	mu      sync.Mutex
	tenants map[string]*tenantTiming
}

// Bound idle retained state. Active callers own their tenant gate until their
// request completes; all discovery uses that request's cancellation context.
func (c *pendingTiming) acquire(ctx context.Context, tenant string) (*tenantTiming, func()) {
	c.mu.Lock()
	if c.tenants == nil {
		c.tenants = make(map[string]*tenantTiming)
	}
	for key, cached := range c.tenants {
		if cached.users == 0 && time.Since(cached.used) > 5*time.Minute {
			delete(c.tenants, key)
		}
	}
	state := c.tenants[tenant]
	if state == nil {
		if len(c.tenants) >= 64 {
			var oldest string
			for key, value := range c.tenants {
				if value.users == 0 && (oldest == "" || value.used.Before(c.tenants[oldest].used)) {
					oldest = key
				}
			}
			if oldest == "" {
				c.mu.Unlock()
				return nil, func() {}
			}
			delete(c.tenants, oldest)
		}
		state = &tenantTiming{gate: make(chan struct{}, 1), entries: make(map[string]timingEntry)}
		c.tenants[tenant] = state
	}
	state.users++
	c.mu.Unlock()
	release := func() { c.mu.Lock(); state.users--; state.used = time.Now(); c.mu.Unlock() }
	select {
	case state.gate <- struct{}{}:
		return state, func() { <-state.gate; release() }
	case <-ctx.Done():
		release()
		return nil, func() {}
	}
}
func summaryVersion(row pendingInputSummary) string {
	data, _ := json.Marshal(row.RecipeJob)
	return string(data)
}

func (s *Server) timedPendingInputs(ctx context.Context, tenant string) ([]pendingInputSummary, error) {
	state, release := s.timing.acquire(ctx, tenant)
	defer release()
	// Read only after acquiring the gate: a delayed older caller cannot revert
	// a newer snapshot or cache entry.
	rows, err := s.pendingInputSummaries(ctx, tenant)
	if err != nil || state == nil {
		return rows, err
	}
	reader, ok := s.inputs.(occurrenceReader)
	if !ok {
		return rows, nil
	}
	now := time.Now()
	current := make(map[string]string, len(rows))
	discover := false
	for _, row := range rows {
		version := summaryVersion(row)
		current[row.ID] = version
		entry, exists := state.entries[row.ID]
		if !exists || entry.version != version {
			entry = timingEntry{version: version}
			state.entries[row.ID] = entry
		}
		if entry.occurrence.JobID == "" && !now.Before(entry.retry) {
			discover = true
		}
	}
	for id := range state.entries {
		if _, exists := current[id]; !exists {
			delete(state.entries, id)
		}
	}
	if discover {
		discoveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		occurrences := make(map[string]input.PendingInputOccurrence)
		token := ""
		for {
			var page input.PendingInputPage
			page, err = reader.ListPendingInputsPage(discoveryCtx, tenant, input.PendingInputOptions{PageSize: jobdb.MaxListJobsPageSize, PageToken: token})
			if err != nil {
				break
			}
			for _, occurrence := range page.Inputs {
				occurrences[occurrence.JobID] = occurrence
			}
			if page.NextPageToken == "" {
				break
			}
			token = page.NextPageToken
		}
		// Discovery is live. Recheck compact versions before associating a form with
		// the candidate; any replacement or removal during discovery loses timing.
		verified, verifyErr := s.pendingInputSummaries(ctx, tenant)
		if verifyErr != nil {
			err = verifyErr
		}
		if verifyErr == nil {
			rows = verified
			current = make(map[string]string, len(rows))
			for _, row := range rows {
				current[row.ID] = summaryVersion(row)
			}
		}
		// A failed verification is a timing failure, not permission to retry
		// discovery on every stream tick. Keep the captured version/backoff.
		versions := current
		for id, entry := range state.entries {
			if versions[id] != entry.version {
				delete(state.entries, id)
				continue
			}
			if entry.occurrence.JobID == "" && !now.Before(entry.retry) {
				entry.retry = now.Add(30 * time.Second)
				if err == nil {
					entry.occurrence = occurrences[id]
				}
				state.entries[id] = entry
			}
		}
	}
	for i := range rows {
		entry := state.entries[rows[i].ID]
		if entry.version != current[rows[i].ID] {
			continue
		}
		occurrence := entry.occurrence
		rows[i].RequestID = occurrence.RequestID
		if occurrence.JobID != "" {
			ordinal := occurrence.TaskOrdinal
			rows[i].TaskOrdinal = &ordinal
		}
		if parsed, parseErr := time.Parse(time.RFC3339Nano, occurrence.RequestedAt); parseErr == nil && !parsed.IsZero() {
			rows[i].RequestedAt = occurrence.RequestedAt
		}
	}

	return rows, nil
}
