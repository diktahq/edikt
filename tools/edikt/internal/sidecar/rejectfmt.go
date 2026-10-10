// rejectfmt.go — a sidecar rejection NAMES THE FIELD.
//
// WHY. A colleague spent two hours on one guideline extraction that then
// failed. One over-length field rejects the ENTIRE sidecar, and the extractor
// was told nothing about which field. Caps only move that wall; naming the
// field removes it.
//
// The underlying library already knows the answer -- it renders
// "at '/directives/0/text': maxLength: got 600, want 500", which carries
// field, index, actual and limit. Two things were losing it:
//
//  1. v1 (compactSchemaError) joined all causes and then cut the result at 137
//     characters. With more than one cause the informative one is frequently
//     the second, so the truncation removed exactly the part the reader needed.
//  2. v2 did not compact at all -- it wrapped the raw multi-line error with %w,
//     so the message spanned several lines, breaking Load's single-line
//     contract, and led with a file:// URL that embeds the HOST ABSOLUTE PATH
//     of the schema.
//
// So the two schema versions reported rejections in two different formats,
// neither of which reliably reached the reader with the field name intact.
// This file is the one renderer for both.
//
// Output contract: ONE line, under 500 characters, every entry naming its
// field. When entries do not fit, WHOLE entries are dropped and the count is
// reported ("+3 more") -- a truncated field name is worse than an absent one,
// because it looks like information.
package sidecar

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// Load's error contract is <500 chars, asserted by the adversarial corpus
	// test. Callers prefix their own tag ("gov-sidecar v2 schema: ",
	// "decode for v2 schema validation: " -- 32 chars at the longest), so the
	// body budget must leave room for it or the assembled string overruns a
	// contract this file believed it was meeting.
	maxRejectionLen = 440
	// Below this, dropping one more entry cannot help; emit the count instead.
	minEntriesShown = 1
)

var (
	reAt = regexp.MustCompile(`^at '([^']*)':\s*(.*)$`)
	// THOUSANDS SEPARATORS. The library renders "got 2,100, want 2,000" once a
	// bound passes 999. Matching only bare digits meant every over-length
	// message fell through to the generic branch the moment the cap moved from
	// 500 to 2000 -- the field name survived, but "is N chars, max M" did not.
	// Invisible while every cap was three digits.
	reMaxLength   = regexp.MustCompile(`^maxLength:\s*got ([\d,]+), want ([\d,]+)$`)
	reMinLength   = regexp.MustCompile(`^minLength:\s*got ([\d,]+), want ([\d,]+)$`)
	reMinItems    = regexp.MustCompile(`^minItems:\s*got ([\d,]+), want ([\d,]+)$`)
	reMaxItems    = regexp.MustCompile(`^maxItems:\s*got ([\d,]+), want ([\d,]+)$`)
	reAddlProps   = regexp.MustCompile(`^additional propert(?:y|ies) (.+) not allowed$`)
	reMissing     = regexp.MustCompile(`^missing propert(?:y|ies) (.+)$`)
	rePattern     = regexp.MustCompile(`^(.*) does not match pattern '(.*)'$`)
	reEnum        = regexp.MustCompile(`^value must be one of (.*)$`)
	reType        = regexp.MustCompile(`^got (\w+), want (\w+)$`)
	reSchemaURLLn = regexp.MustCompile(`^jsonschema validation failed`)
)

// pointerToField renders an RFC-6901 JSON pointer the way the author wrote the
// document: "/directives/0/text" -> "directives[0].text". A reader hunting a
// field in their YAML should not have to translate a pointer first.
func pointerToField(ptr string) string {
	ptr = strings.TrimPrefix(ptr, "/")
	if ptr == "" {
		return "(document root)"
	}
	var b strings.Builder
	for i, seg := range strings.Split(ptr, "/") {
		// RFC-6901 escapes, in the order the spec mandates.
		seg = strings.ReplaceAll(seg, "~1", "/")
		seg = strings.ReplaceAll(seg, "~0", "~")
		if _, err := strconv.Atoi(seg); err == nil {
			b.WriteString("[" + seg + "]")
			continue
		}
		if i > 0 {
			b.WriteString(".")
		}
		b.WriteString(seg)
	}
	return b.String()
}

// plainNum strips the library's thousands separators so a reported number can
// be compared, grepped, and pasted without editing.
func plainNum(s string) string { return strings.ReplaceAll(s, ",", "") }

// explain turns one library cause into a sentence whose subject is the field.
func explain(field, reason string) string {
	switch {
	case reMaxLength.MatchString(reason):
		m := reMaxLength.FindStringSubmatch(reason)
		return fmt.Sprintf("%s is %s chars, max %s", field, plainNum(m[1]), plainNum(m[2]))
	case reMinLength.MatchString(reason):
		m := reMinLength.FindStringSubmatch(reason)
		return fmt.Sprintf("%s is %s chars, min %s", field, plainNum(m[1]), plainNum(m[2]))
	case reMinItems.MatchString(reason):
		m := reMinItems.FindStringSubmatch(reason)
		return fmt.Sprintf("%s has %s entries, min %s", field, plainNum(m[1]), plainNum(m[2]))
	case reMaxItems.MatchString(reason):
		m := reMaxItems.FindStringSubmatch(reason)
		return fmt.Sprintf("%s has %s entries, max %s", field, plainNum(m[1]), plainNum(m[2]))
	case reAddlProps.MatchString(reason):
		m := reAddlProps.FindStringSubmatch(reason)
		return fmt.Sprintf("%s has unknown key %s (this level accepts no extra keys)", field, m[1])
	case reMissing.MatchString(reason):
		m := reMissing.FindStringSubmatch(reason)
		return fmt.Sprintf("%s is missing required key %s", field, m[1])
	case rePattern.MatchString(reason):
		m := rePattern.FindStringSubmatch(reason)
		return fmt.Sprintf("%s value %s does not match required pattern %s", field, m[1], m[2])
	case reEnum.MatchString(reason):
		m := reEnum.FindStringSubmatch(reason)
		return fmt.Sprintf("%s must be one of %s", field, m[1])
	case reType.MatchString(reason):
		m := reType.FindStringSubmatch(reason)
		return fmt.Sprintf("%s is %s, want %s", field, m[1], m[2])
	default:
		// Unrecognised kinds still lead with the field. A rule this file has
		// not learned to phrase must not silently become an anonymous error.
		return fmt.Sprintf("%s: %s", field, reason)
	}
}

// formatSchemaErrors renders a validation error as ONE field-naming line.
// truncateAtBoundary cuts a too-long single entry at the last list delimiter
// that fits, never mid-token.
//
// The library reports N unknown keys at one level as ONE comma-joined cause,
// so "drop whole entries" is not enough on its own: one entry can exceed the
// whole budget. Cutting it at a fixed offset re-creates the defect this file
// exists to remove -- a half-written field name that reads as information.
func truncateAtBoundary(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const suffix = " ...(list truncated)"
	cut := max - len(suffix)
	if cut < 1 {
		return s[:max]
	}
	window := s[:cut]
	if i := strings.LastIndex(window, ", "); i > 0 {
		return window[:i] + suffix
	}
	if i := strings.LastIndex(window, "; "); i > 0 {
		return window[:i] + suffix
	}
	return window + suffix
}

func formatSchemaErrors(err error) string {
	var entries []string
	for _, ln := range strings.Split(err.Error(), "\n") {
		ln = strings.TrimSpace(ln)
		ln = strings.TrimPrefix(ln, "- ")
		ln = strings.TrimSpace(ln)
		// Drops the "jsonschema validation failed with 'file://...'" header,
		// which names where the SCHEMA lives -- noise to someone whose sidecar
		// will not load, and it embeds the host absolute path.
		if ln == "" || reSchemaURLLn.MatchString(ln) {
			continue
		}
		if m := reAt.FindStringSubmatch(ln); m != nil {
			entries = append(entries, explain(pointerToField(m[1]), strings.TrimSpace(m[2])))
			continue
		}
		entries = append(entries, ln)
	}
	if len(entries) == 0 {
		return "validation failed (no cause reported)"
	}
	// Drop WHOLE entries rather than cutting mid-field. A half-written field
	// name reads as information and is not.
	shown := entries
	for len(shown) > minEntriesShown {
		if len(strings.Join(shown, "; "))+16 <= maxRejectionLen {
			break
		}
		shown = shown[:len(shown)-1]
	}
	msg := strings.Join(shown, "; ")
	if n := len(entries) - len(shown); n > 0 {
		msg += fmt.Sprintf(" (+%d more)", n)
	}
	return truncateAtBoundary(msg, maxRejectionLen)
}

// formatYAMLError renders a yaml.v3 decode failure as ONE bounded line.
//
// Found 2026-08-30 by the many-errors control: a document with duplicate keys
// produced 1,659 characters across 15 lines, straight out of the library and
// through both validators untouched. Schema rejections were being formatted
// while DECODE rejections -- which a hand-edited sidecar hits at least as
// often, via a duplicate key or bad indentation -- were not. Same contract as
// formatSchemaErrors: one line, bounded, whole entries dropped and counted.
func formatYAMLError(err error) string {
	var entries []string
	for _, ln := range strings.Split(err.Error(), "\n") {
		ln = strings.TrimSpace(ln)
		// "yaml: unmarshal errors:" is a header, not a cause.
		if ln == "" || strings.HasSuffix(ln, "unmarshal errors:") {
			continue
		}
		entries = append(entries, strings.TrimPrefix(ln, "yaml: "))
	}
	if len(entries) == 0 {
		return "YAML decode failed (no cause reported)"
	}
	shown := entries
	for len(shown) > minEntriesShown {
		if len(strings.Join(shown, "; "))+16 <= maxRejectionLen {
			break
		}
		shown = shown[:len(shown)-1]
	}
	msg := strings.Join(shown, "; ")
	if n := len(entries) - len(shown); n > 0 {
		msg += fmt.Sprintf(" (+%d more)", n)
	}
	return truncateAtBoundary(msg, maxRejectionLen)
}
