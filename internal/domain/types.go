// Package domain holds shared entities for gesetzeswache.
// Entities: Law, LawVariant, StandCitation, GazetteIssue, IssueLawLink,
// FreshnessRecord, SyncAttempt.
package domain

import "time"

// FreshnessState is the verified freshness of a law relative to BGBl.
type FreshnessState string

const (
	FreshnessConfirmedCurrent FreshnessState = "confirmed_current"
	FreshnessConfirmedStale   FreshnessState = "confirmed_stale"
	FreshnessUncertain        FreshnessState = "uncertain"
)

// LinkClass distinguishes confirmed GII feed links from heuristics.
type LinkClass string

const (
	LinkConfirmed LinkClass = "confirmed"
	LinkHeuristic LinkClass = "heuristic"
)

// VerificationMethod describes how existence/freshness evidence was obtained.
type VerificationMethod string

const (
	MethodFeeds     VerificationMethod = "feeds"
	MethodProbeOnly VerificationMethod = "probe_only"
	MethodMixed     VerificationMethod = "mixed"
)

// Law is a federal statute identity (metadata only; no full text).
type Law struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Title        string `json:"title"`
	GIIPath      string `json:"gii_path"` // relative path / slug on GII
	GIIURL       string `json:"gii_url"`
}

// LawVariant maps an informal/alternate name to a canonical law.
type LawVariant struct {
	Variant string `json:"variant"`
	LawID   string `json:"law_id"`
}

// InstrumentRef is a BGBl citation extracted from Stand or +++ editorial notes.
type InstrumentRef struct {
	Kind        string `json:"kind,omitempty"` // G|V|Bek|""
	Teil        int    `json:"teil,omitempty"`
	Year        int    `json:"year,omitempty"`
	Number      string `json:"number,omitempty"`
	SectionHint string `json:"section_hint,omitempty"` // e.g. "§ 1"
	Raw         string `json:"raw,omitempty"`
}

// VRefResolution records how an operative instrument ref was matched to linked children.
type VRefResolution struct {
	Ref            InstrumentRef `json:"ref"`
	MatchedGIISlug string        `json:"matched_gii_slug,omitempty"`
	MatchMethod    string        `json:"match_method,omitempty"` // notes_identity | bgbl_index | ""
	ChildConfirmed bool          `json:"child_confirmed,omitempty"`
	Resolved       bool          `json:"resolved,omitempty"`
	Historical     bool          `json:"historical,omitempty"` // past-chain only (non-V); ignore for unresolved
}

// LinkedInstrument maps a parent law to a related ordinance/instrument (seeded TSV).
type LinkedInstrument struct {
	ParentLawID   string `json:"parent_law_id"`
	Kind          string `json:"kind"` // verordnung|gesetz|…
	GIISlug       string `json:"gii_slug"`
	Notes         string `json:"notes,omitempty"`
	EffectiveFrom string `json:"effective_from,omitempty"` // YYYY-MM-DD Inkrafttreten
	SectionHint   string `json:"section_hint,omitempty"`
	Status        string `json:"status,omitempty"`     // past|current|future
	Coverage      string `json:"coverage,omitempty"`   // "section" default
	Source        string `json:"source,omitempty"`     // seeded | discovered
	Confidence    string `json:"confidence,omitempty"` // high | medium | low
	EdgeType      string `json:"edge_type,omitempty"`  // ermaechtigung | bgbl_plus_plus
	// Pointer fields (filled when include=linked):
	ResolveOK bool   `json:"resolve_ok,omitempty"`
	GIIURL    string `json:"gii_url,omitempty"`
	LawID     string `json:"law_id,omitempty"`
}

// DiscoveredEdge is a persisted parent→child instrument link from discovery.
type DiscoveredEdge struct {
	ParentLawID   string `json:"parent_law_id"`
	GIISlug       string `json:"gii_slug"`
	SectionHint   string `json:"section_hint,omitempty"`
	Notes         string `json:"notes,omitempty"`
	EdgeType      string `json:"edge_type"`
	Confidence    string `json:"confidence"`
	EffectiveFrom string `json:"effective_from,omitempty"`
	ChildLawID    string `json:"child_law_id,omitempty"`
}

// StandCitation is the GII "Stand" signal, raw and parsed.
type StandCitation struct {
	LawID          string     `json:"law_id"`
	Raw            string     `json:"raw"`
	Teil           int        `json:"teil,omitempty"` // 1 or 2
	Year           int        `json:"year,omitempty"`
	Number         string     `json:"number,omitempty"` // issue number, may include letter suffix
	Page           string     `json:"page,omitempty"`
	Date           *time.Time `json:"date,omitempty"`
	InstrumentKind string     `json:"instrument_kind,omitempty"` // G|V|Bek from Stand text
	ParseOK        bool       `json:"parse_ok"`
	ParseNotes     string     `json:"parse_notes,omitempty"`
}

// GazetteIssue is an observed BGBl promulgation.
type GazetteIssue struct {
	ID                  string     `json:"id"` // e.g. "BGBl-1/2026/209"
	Teil                int        `json:"teil"`
	Year                int        `json:"year"`
	Number              string     `json:"number"`
	PublishedAt         *time.Time `json:"published_at,omitempty"`
	Title               string     `json:"title,omitempty"`
	ELIURL              string     `json:"eli_url,omitempty"`
	DiscoverySources    []string   `json:"discovery_sources,omitempty"`
	ExistenceConfidence string     `json:"existence_confidence"` // high|low
	Matched             bool       `json:"matched"`
	FirstSeenAt         time.Time  `json:"first_seen_at"`
}

// IssueLawLink connects a gazette issue to an affected law.
type IssueLawLink struct {
	IssueID   string    `json:"issue_id"`
	LawID     string    `json:"law_id"`
	Class     LinkClass `json:"class"`
	CreatedAt time.Time `json:"created_at"`
}

// FreshnessRecord is the current freshness evaluation for a law.
type FreshnessRecord struct {
	LawID         string             `json:"law_id"`
	State         FreshnessState     `json:"state"`
	Confidence    string             `json:"confidence"` // high|medium|low
	Method        VerificationMethod `json:"method"`
	EvaluatedAt   time.Time          `json:"evaluated_at"`
	NewerIssueIDs []string           `json:"newer_issue_ids,omitempty"`
	Rationale     string             `json:"rationale,omitempty"`
}

// SyncAttempt records one background sync job outcome.
type SyncAttempt struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // toc|stand|gii_feed|bgbl_feed|eli_probe
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// SyncStatus summarizes last successful sync times per source.
type SyncStatus struct {
	CatalogReady        bool         `json:"catalog_ready"`
	LastTOCSuccess      *time.Time   `json:"last_toc_success,omitempty"`
	LastGIIFeedSuccess  *time.Time   `json:"last_gii_feed_success,omitempty"`
	LastBGBlFeedSuccess *time.Time   `json:"last_bgbl_feed_success,omitempty"`
	LastELIProbeSuccess *time.Time   `json:"last_eli_probe_success,omitempty"`
	LastReconcileAt     *time.Time   `json:"last_reconcile_at,omitempty"`
	DataFresh           bool         `json:"data_fresh"` // within max-age for confirmed claims
	MaxAge              DurationJSON `json:"max_age"`
}

// DurationJSON is a duration exposed as string in JSON helpers elsewhere.
type DurationJSON time.Duration
