package expression

import (
	"regexp"
	"strings"
)

var whitespaceRegex = regexp.MustCompile(`\s+`)

// Preprocess applies preprocessing transformations to an expression string
// before parsing. This includes:
// - Normalizing symbolic operators to canonical forms
// - Canonicalizing legacy fused negations
// - Relocating pre-field modifiers
// - Collapsing excess whitespace
func Preprocess(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	// 1. Normalize symbolic operators
	result := normalizeSymbolicOperators(raw)

	// 2. Canonicalize legacy fused negations
	result = canonicalizeLegacyFusedNegations(result)

	// 3. Relocate pre-field modifiers
	result = relocatePreFieldModifiers(result)

	// 4. Collapse excess whitespace
	result = collapseWhitespace(result)

	return result
}

// normalizeSymbolicOperators converts symbolic operators to canonical snake_case
// operator tokens. Negations are expressed via separate 'not' modifier.
//
// Symbol mappings:
//
//	==  -> equals
//	!=  -> not equals
//	=~  -> matches
//	!~  -> not matches
//	>=  -> greater_than_or_equal
//	<=  -> less_than_or_equal
//	>   -> greater_than
//	<   -> less_than
//	&&  -> AND
//	||  -> OR
func normalizeSymbolicOperators(input string) string {
	s := input

	// Order matters: longer patterns first to avoid partial replacements
	replacements := []struct {
		pattern     string
		replacement string
	}{
		{"!~", " not matches "},
		{"=~", " matches "},
		{"!=", " not equals "},
		{"==", " equals "},
		{">=", " greater_than_or_equal "},
		{"<=", " less_than_or_equal "},
		{">", " greater_than "},
		{"<", " less_than "},
	}

	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.pattern, r.replacement)
	}

	// Logical operator normalization
	s = strings.ReplaceAll(s, "&&", " AND ")
	s = strings.ReplaceAll(s, "||", " OR ")

	// Normalize lowercase textual variants
	s = strings.ReplaceAll(s, " and ", " AND ")
	s = strings.ReplaceAll(s, " or ", " OR ")

	return s
}

// canonicalizeLegacyFusedNegations converts fused negated operator tokens
// (e.g., not_equals) to the preferred modifier + operator form (not equals).
func canonicalizeLegacyFusedNegations(input string) string {
	mappings := []struct {
		from string
		to   string
	}{
		{" not_equals ", " not equals "},
		{" not_matches ", " not matches "},
		{" not_contains ", " not contains "},
		{" not_starts_with ", " not starts_with "},
		{" not_ends_with ", " not ends_with "},
	}

	result := input
	for _, m := range mappings {
		result = strings.ReplaceAll(result, m.from, m.to)
	}

	return result
}

// relocatePreFieldModifiers moves legacy pre-field modifiers to mid-field form.
// For example: "not field contains" -> "field not contains"
func relocatePreFieldModifiers(input string) string {
	trimmed := strings.TrimLeft(input, " \t")

	// Quick check: if it doesn't start with a modifier keyword, skip
	preModStarters := []string{"not ", "case_sensitive "}
	startsWithMod := false
	for _, starter := range preModStarters {
		if strings.HasPrefix(trimmed, starter) {
			startsWithMod = true
			break
		}
	}
	if !startsWithMod {
		return input
	}

	// Parse leading modifiers and field
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		return input
	}

	var modifiers []string
	var fieldIdx int

	for i, token := range parts {
		if token == "not" || token == "case_sensitive" {
			modifiers = append(modifiers, token)
		} else {
			fieldIdx = i
			break
		}
	}

	if len(modifiers) == 0 || fieldIdx >= len(parts) {
		return input
	}

	field := parts[fieldIdx]
	rest := parts[fieldIdx+1:]

	// Rebuild: field <mods> rest_of_expression
	var rebuilt strings.Builder
	rebuilt.WriteString(field)
	rebuilt.WriteByte(' ')
	rebuilt.WriteString(strings.Join(modifiers, " "))
	if len(rest) > 0 {
		rebuilt.WriteByte(' ')
		rebuilt.WriteString(strings.Join(rest, " "))
	}

	// Preserve leading whitespace
	leadingWs := input[:len(input)-len(trimmed)]
	return leadingWs + rebuilt.String()
}

// collapseWhitespace reduces multiple consecutive whitespace characters to single spaces
// while trimming leading and trailing whitespace.
func collapseWhitespace(input string) string {
	return strings.TrimSpace(whitespaceRegex.ReplaceAllString(input, " "))
}

// PreprocessAndParse preprocesses an expression string and then parses it.
// This is a convenience function that combines preprocessing and parsing.
func PreprocessAndParse(raw string) (*ParsedExpression, error) {
	preprocessed := Preprocess(raw)
	if preprocessed == "" {
		return nil, nil
	}
	return Parse(preprocessed)
}
