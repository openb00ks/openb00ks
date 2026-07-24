package pipeline

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// VendorRef is a candidate vendor offered to the matcher. DefaultAccount is the vendor's memoized
// expense account (empty if none) — when a receipt matches this vendor, the pipeline uses it and skips
// the classify AI call. MatchPattern is the distinctive substring learned for this vendor (e.g.
// "BLUE BOTTLE"), used by candidate retrieval to catch messy receipt strings the display name misses.
type VendorRef struct {
	ID             string
	Name           string
	TaxID          string
	MatchPattern   string
	DefaultAccount string
}

// ProposedVendor is the AI's recommendation for a vendor that isn't in the candidate list — enough to
// create one in the system (a vendor_rule: MatchPattern → the classified account). Name is a cleaned
// canonical display name (receipts print messy strings like "SQ *BLUE BOTTLE"); MatchPattern is a
// distinctive substring expected on this vendor's future receipts (used for auto-matching). TaxID and
// Website are filled only when printed on the receipt.
type ProposedVendor struct {
	Name         string  `json:"name"`
	MatchPattern string  `json:"match_pattern"`
	TaxID        *string `json:"tax_id"`
	Website      *string `json:"website"`
}

// VendorMatchResult is the vendor-match stage output. Exactly one of VendorID (matched an existing
// candidate) or ProposedVendor (a recommended new vendor) is populated on a confident result.
type VendorMatchResult struct {
	VendorID       *string         `json:"vendor_id"`
	IsNewVendor    bool            `json:"is_new_vendor"`
	ProposedVendor *ProposedVendor `json:"proposed_vendor"`
	Confidence     float64         `json:"confidence"`
	Reason         string          `json:"reason"`
}

// NormalizeVendorName folds a vendor string for exact comparison: lowercased, alphanumerics only, with
// common company suffixes removed. Deterministic — no model needed.
func NormalizeVendorName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	n := b.String()
	for _, suffix := range []string{"inc", "llc", "ltd", "co", "corp", "company"} {
		n = strings.TrimSuffix(n, suffix)
	}
	return n
}

// ExactVendorMatch is the deterministic first pass (dedup phase A): a normalized-name equality
// against the candidates. Returns the matching vendor id, or nil for no exact match — only then does
// the orchestration call the AI adjudicator.
func ExactVendorMatch(rawVendor string, candidates []VendorRef) *string {
	want := NormalizeVendorName(rawVendor)
	if want == "" {
		return nil
	}
	for _, c := range candidates {
		if NormalizeVendorName(c.Name) == want {
			id := c.ID
			return &id
		}
	}
	return nil
}

const vendorSystem = `You match a raw vendor name from a receipt to one of the candidate vendors below,
or, if none matches (or there are no candidates), RECOMMEND a new vendor. Rules:
- If a candidate is clearly the same vendor: set vendor_id to its id (verbatim from the list),
  is_new_vendor=false, proposed_vendor=null.
- Otherwise: set vendor_id=null, is_new_vendor=true, and fill proposed_vendor so the system can create
  the vendor:
    - name: a CLEAN canonical vendor name (fix messy receipt formatting, e.g. "SQ *BLUE BOTTLE" →
      "Blue Bottle Coffee"; drop store numbers, POS prefixes, and payment-processor noise).
    - match_pattern: a short, distinctive UPPERCASE substring that will appear on THIS vendor's future
      receipts (used to auto-match later). Pick something specific, not a generic word.
    - tax_id / website: only if printed on the receipt, else null.
- A wrong match misfiles every future receipt from this vendor — be CONSERVATIVE. When unsure, prefer a
  new-vendor recommendation over a doubtful match, and lower your confidence.
Return ONLY valid JSON matching the schema.

Candidate vendors:
`

// VendorSchema is the strict schema for the vendor-match stage.
const VendorSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["vendor_id","is_new_vendor","proposed_vendor","confidence","reason"],
  "properties": {
    "vendor_id": {"type": ["string","null"]},
    "is_new_vendor": {"type": "boolean"},
    "proposed_vendor": {
      "type": ["object","null"],
      "additionalProperties": false,
      "required": ["name","match_pattern","tax_id","website"],
      "properties": {
        "name": {"type": "string"},
        "match_pattern": {"type": "string"},
        "tax_id": {"type": ["string","null"]},
        "website": {"type": ["string","null"]}
      }
    },
    "confidence": {"type": "number"},
    "reason": {"type": "string"}
  }
}`

// BuildVendorMatchRequest builds the (system, user, schema) for AI adjudication of a pre-retrieved
// candidate shortlist (the model judges, it does not search).
func BuildVendorMatchRequest(rawVendor string, candidates []VendorRef) (system, user, schema string) {
	var b strings.Builder
	b.WriteString(vendorSystem)
	for _, c := range candidates {
		if c.TaxID != "" {
			fmt.Fprintf(&b, "- id=%s name=%q tax_id=%s\n", c.ID, c.Name, c.TaxID)
		} else {
			fmt.Fprintf(&b, "- id=%s name=%q\n", c.ID, c.Name)
		}
	}
	return b.String(), "Raw vendor from receipt: " + rawVendor, VendorSchema
}

// ParseVendorMatch decodes the model response.
func ParseVendorMatch(raw string) (VendorMatchResult, error) {
	var r VendorMatchResult
	if err := json.Unmarshal([]byte(stripCodeFence(raw)), &r); err != nil {
		return VendorMatchResult{}, fmt.Errorf("vendor-match: invalid JSON: %w", err)
	}
	return r, nil
}

// ValidateVendorMatch enforces consistency: a returned id must be one that was offered, and the
// new-vendor flag must agree with the id.
func ValidateVendorMatch(r VendorMatchResult, candidates []VendorRef) []string {
	var issues []string
	if r.Confidence < 0 || r.Confidence > 1 {
		issues = append(issues, fmt.Sprintf("confidence %.2f out of range", r.Confidence))
	}
	switch {
	case r.VendorID != nil && r.IsNewVendor:
		issues = append(issues, "vendor_id set but is_new_vendor is true")
	case r.VendorID != nil:
		found := false
		for _, c := range candidates {
			if c.ID == *r.VendorID {
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, fmt.Sprintf("vendor_id %q is not in the candidate list", *r.VendorID))
		}
	case r.IsNewVendor:
		// A new-vendor recommendation must carry enough to create one.
		switch {
		case r.ProposedVendor == nil:
			issues = append(issues, "is_new_vendor but proposed_vendor is null")
		case strings.TrimSpace(r.ProposedVendor.Name) == "":
			issues = append(issues, "proposed_vendor.name is empty")
		case strings.TrimSpace(r.ProposedVendor.MatchPattern) == "":
			issues = append(issues, "proposed_vendor.match_pattern is empty")
		}
	default:
		// No id and not flagged new → undecided; don't silently proceed.
		issues = append(issues, "no vendor_id and is_new_vendor is false (undecided)")
	}
	return issues
}

// GateVendorMatch applies the confidence gate + validation. A confident new-vendor decision is a valid
// advance (the orchestration then creates the vendor).
func GateVendorMatch(r VendorMatchResult, candidates []VendorRef) Outcome {
	return Gate(r.Confidence, VendorConfidenceMin, ValidateVendorMatch(r, candidates))
}

// vendorCandidateMinScore filters out weak fuzzy matches from the shortlist.
const vendorCandidateMinScore = 0.2

// RankVendorCandidates orders known vendors by similarity to the raw receipt vendor string and returns
// the top `limit` with real signal — the deterministic shortlist handed to the vendor-match model
// (recall over precision; the model makes the final call). Pure and testable.
func RankVendorCandidates(query string, candidates []VendorRef, limit int) []VendorRef {
	qn := NormalizeVendorName(query)
	if qn == "" || len(candidates) == 0 {
		return nil
	}
	type scored struct {
		ref   VendorRef
		score float64
	}
	var ranked []scored
	for _, c := range candidates {
		if s := vendorScore(qn, c); s >= vendorCandidateMinScore {
			ranked = append(ranked, scored{c, s})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]VendorRef, len(ranked))
	for i, r := range ranked {
		out[i] = r.ref
	}
	return out
}

// vendorScore scores a normalized receipt vendor string against a candidate, taking the stronger of its
// name similarity and its learned match_pattern. A distinctive pattern appearing verbatim in the receipt
// string (e.g. "bluebottle" inside "sqbluebottle1234") is a high-confidence signal the display name alone
// would miss — this is what makes memoization actually re-match a vendor's messy future receipts.
func vendorScore(queryNorm string, c VendorRef) float64 {
	s := vendorSimilarity(queryNorm, NormalizeVendorName(c.Name))
	if pn := NormalizeVendorName(c.MatchPattern); pn != "" {
		if strings.Contains(queryNorm, pn) {
			s = math.Max(s, 0.9)
		} else {
			s = math.Max(s, trigramJaccard(queryNorm, pn))
		}
	}
	return s
}

// vendorSimilarity scores two ALREADY-normalized vendor names in [0,1].
func vendorSimilarity(a, b string) float64 {
	switch {
	case a == "" || b == "":
		return 0
	case a == b:
		return 1
	case strings.Contains(a, b) || strings.Contains(b, a):
		return 0.8
	default:
		return trigramJaccard(a, b)
	}
}

func trigramJaccard(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func trigrams(s string) map[string]bool {
	m := map[string]bool{}
	r := []rune(s)
	if len(r) < 3 {
		if s != "" {
			m[s] = true
		}
		return m
	}
	for i := 0; i+3 <= len(r); i++ {
		m[string(r[i:i+3])] = true
	}
	return m
}
