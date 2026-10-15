// Package source implements the two observation channels used to monitor
// tinyproxy: its StatHost statistics page and its log file.
//
// Everything in this file is derived from tinyproxy 1.11.3 sources:
//
//   - src/stats.c            struct stat_s holds only num_reqs, num_badcons,
//     num_open, num_refused, num_denied. There is *no* byte
//     counter anywhere in tinyproxy.
//   - src/stats.c:119-127    with a StatFile configured, tinyproxy substitutes
//     {opens} {reqs} {badconns} {deniedconns} {refusedconns}
//     plus the standard variables.
//   - src/html-error.c:126   the substitution pattern is {[a-z]{1,32}}.
//   - src/stats.c:84-107     without a StatFile the built-in page prints
//     "Number of open connections: N<br />" and friends.
//   - data/templates/stats.html  the shipped template renders a table with the
//     labels "Open connections", "Total requests", ...
//
// The parser therefore understands all three shapes and additionally mirrors
// every numeric label it sees into a generic passthrough field, so a future
// tinyproxy release cannot silently blind the exporter.
package source

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Canonical field names produced by ParseStatsPage. They are mapped to metric
// names by the collector package.
const (
	FieldOpenConnections    = "open_connections"
	FieldRequestsTotal      = "requests_total"
	FieldBadConnections     = "bad_connections"
	FieldDeniedConnections  = "denied_connections"
	FieldRefusedConnections = "refused_connections"
)

// Canonical text fields produced by ParseStatsPage.
const (
	TextFieldVersion = "version"
	TextFieldPackage = "package"
)

// monotonicFields lists the fields that only ever grow while a tinyproxy
// process is alive. They can be exposed as Prometheus counters once the
// occasional restart has been compensated for.
var monotonicFields = map[string]bool{
	FieldRequestsTotal:      true,
	FieldBadConnections:     true,
	FieldDeniedConnections:  true,
	FieldRefusedConnections: true,
}

// StatsResult is a parsed statistics page.
type StatsResult struct {
	// Fields holds canonically named numeric values (see Field* constants).
	Fields map[string]float64
	// Text holds canonically named string values (see TextField* constants).
	Text map[string]string
	// Raw mirrors every numeric label exactly as it appeared on the page.
	Raw map[string]float64
	// RawText mirrors every textual label exactly as it appeared.
	RawText map[string]string

	pendingLabel string // line oriented pairing state, see consumeLine
}

func newStatsResult() *StatsResult {
	return &StatsResult{
		Fields:  make(map[string]float64),
		Text:    make(map[string]string),
		Raw:     make(map[string]float64),
		RawText: make(map[string]string),
	}
}

var (
	reStripBlock = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</\s*(?:script|style)\s*>`)
	reLineBreak  = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>|</\s*(?:p|div|tr|li|h[1-6]|td|th|dt|dd|pre|table|body|html|head|title)\s*>`)
	reAnyTag     = regexp.MustCompile(`<[^>]*>`)
	reWhitespace = regexp.MustCompile(`[\s\x{00a0}]+`)

	// "Label: 12" / "label = 12" and "label 12" both occur in the wild.
	reSeparatorKV = regexp.MustCompile(`^([^:=]+?)\s*[:=]\s*([-+]?[0-9]+(?:\.[0-9]+)?)\s*$`)
	reSplitKV     = regexp.MustCompile(`^([A-Za-z][^:=]*?)\s+([-+]?[0-9]+(?:\.[0-9]+)?)\s*$`)
	// Our statpage.txt convention: "<key>_text <string>". Checked first because
	// such values are not numbers (a version like 1.11.3 would not match the
	// strict numeric patterns above).
	reTextKV = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.-]*_text)\s+(\S.*?)\s*$`)

	reNumeric  = regexp.MustCompile(`^[-+]?[0-9]+(?:\.[0-9]+)?$`)
	reLabelish = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _()/\-.,%]*$`)
	reLabelSep = regexp.MustCompile(`[^a-z0-9]+`)
)

// ParseStatsPage parses a statistics page body. It never fails: unrecognised
// content is simply not recorded.
func ParseStatsPage(body []byte) *StatsResult {
	res := newStatsResult()
	for _, line := range statsPageLines(string(body)) {
		res.consumeLine(line)
	}
	return res
}

// statsPageLines flattens HTML or plain text into trimmed, non empty lines.
func statsPageLines(raw string) []string {
	s := reStripBlock.ReplaceAllString(raw, "\n")
	s = reLineBreak.ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, "\n")
	s = html.UnescapeString(s)

	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(reWhitespace.ReplaceAllString(p, " "))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// consumeLine feeds one line of the flattened page into the result.
//
// Two layouts are supported:
//   - label and value on the same line ("Number of requests: 42")
//   - label on one line and its value on the next (the shipped table template)
func (r *StatsResult) consumeLine(line string) {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return
	}
	if strings.ContainsAny(line, "{}") {
		// A placeholder tinyproxy did not substitute (unknown variable). Drop
		// the whole line instead of inventing data.
		return
	}
	if m := reTextKV.FindStringSubmatch(line); m != nil {
		r.recordText(m[1], m[2])
		r.pendingLabel = ""
		return
	}
	if key, val, ok := splitKeyValue(line); ok {
		r.record(key, val)
		return
	}
	if reNumeric.MatchString(line) {
		if r.pendingLabel != "" {
			r.record(r.pendingLabel, line)
			r.pendingLabel = ""
		}
		return
	}
	if reLabelish.MatchString(line) && !strings.ContainsAny(line, ":") {
		r.pendingLabel = line
		return
	}
	r.pendingLabel = ""
}

func splitKeyValue(line string) (string, string, bool) {
	if m := reSeparatorKV.FindStringSubmatch(line); m != nil {
		return strings.TrimSpace(m[1]), m[2], true
	}
	if m := reSplitKV.FindStringSubmatch(line); m != nil {
		return strings.TrimSpace(m[1]), m[2], true
	}
	return "", "", false
}

// recordText stores a textual key/value pair (see reTextKV).
func (r *StatsResult) recordText(key, val string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if field, ok := canonicalTextField(strings.ToLower(key)); ok {
		r.Text[field] = val
	}
	if label := sanitizeLabel(key); label != "" {
		r.RawText[label] = val
	}
}

func (r *StatsResult) record(key, val string) {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "{}") {
		return
	}
	lower := strings.ToLower(key)

	if strings.HasSuffix(lower, "_text") || !reNumeric.MatchString(val) {
		if field, ok := canonicalTextField(lower); ok {
			r.Text[field] = val
		}
		if label := sanitizeLabel(key); label != "" {
			r.RawText[label] = val
		}
		return
	}

	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return
	}
	if field, ok := canonicalField(lower); ok {
		r.Fields[field] = f
	}
	if label := sanitizeLabel(key); label != "" {
		r.Raw[label] = f
	}
}

// canonicalField maps a page label to a canonical numeric field name.
// Order matters: "refused" is checked before "request" because
// "Number of refused connections due to high load" would otherwise not match
// any rule, and "bad"/"denied" before the generic "connection" wording.
func canonicalField(label string) (string, bool) {
	l := normalizeLabel(label)
	switch {
	case strings.Contains(l, "refused"), strings.Contains(l, "high load"):
		return FieldRefusedConnections, true
	case strings.Contains(l, "denied"):
		return FieldDeniedConnections, true
	case strings.Contains(l, "bad"):
		return FieldBadConnections, true
	case strings.Contains(l, "request"):
		return FieldRequestsTotal, true
	case strings.Contains(l, "open"), strings.Contains(l, "active"), strings.Contains(l, "current"):
		return FieldOpenConnections, true
	}
	return "", false
}

func canonicalTextField(label string) (string, bool) {
	l := normalizeLabel(label)
	switch {
	case strings.Contains(l, "version"):
		return TextFieldVersion, true
	case strings.Contains(l, "package"):
		return TextFieldPackage, true
	}
	return "", false
}

// normalizeLabel lowercases a label, drops the metric prefixes we ship in
// statpage.txt and collapses separators into single spaces.
func normalizeLabel(label string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	for _, prefix := range []string{"tinyproxy_stathost_", "tinyproxy_stats_", "tinyproxy_"} {
		if strings.HasPrefix(l, prefix) {
			l = strings.TrimPrefix(l, prefix)
			break
		}
	}
	l = reLabelSep.ReplaceAllString(l, " ")
	return strings.TrimSpace(reWhitespace.ReplaceAllString(l, " "))
}

// sanitizeLabel turns an arbitrary label into a safe Prometheus label value.
func sanitizeLabel(label string) string {
	l := reLabelSep.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "_")
	l = strings.Trim(l, "_")
	if l == "" {
		return ""
	}
	const maxLabel = 80
	if len(l) > maxLabel {
		l = l[:maxLabel]
	}
	return l
}
