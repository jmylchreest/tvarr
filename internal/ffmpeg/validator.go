package ffmpeg

import (
	"regexp"
	"strings"
	"unicode"
)

// FlagValidationResult contains the result of validating custom FFmpeg flags
type FlagValidationResult struct {
	Valid       bool     `json:"valid"`
	Flags       []string `json:"flags"`
	Warnings    []string `json:"warnings,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// ValidationSeverity indicates the severity of a validation issue
type ValidationSeverity int

const (
	SeverityInfo ValidationSeverity = iota
	SeverityWarning
	SeverityError
)

// dangerousPatterns are patterns that indicate potential shell injection
var dangerousPatterns = []struct {
	pattern *regexp.Regexp
	message string
}{
	{regexp.MustCompile(`\$\(`), "Command substitution $(...) detected"},
	{regexp.MustCompile("`"), "Backtick command substitution detected"},
	{regexp.MustCompile(`\$\{`), "Variable expansion ${...} detected"},
	{regexp.MustCompile(`\$[A-Za-z_]`), "Variable reference $VAR detected"},
	{regexp.MustCompile(`;`), "Command separator (;) detected"},
	// Note: Single pipe detection handled separately in containsDangerousPipe() to avoid negative lookahead
	{regexp.MustCompile(`&&`), "Command chaining (&&) detected"},
	{regexp.MustCompile(`>>`), "Append redirection (>>) detected"},
	{regexp.MustCompile(`(?:^|[^>])>`), "Output redirection (>) detected"},
	{regexp.MustCompile(`<`), "Input redirection (<) detected"},
}

// blockedFlags are flags that should never be in custom options
var blockedFlags = map[string]string{
	"-i":                  "Input is controlled separately",
	"-y":                  "Overwrite mode is controlled separately",
	"-n":                  "No-overwrite mode is controlled separately",
	"-filter_script":      "Could load arbitrary script files",
	"-filter_script:v":    "Could load arbitrary script files",
	"-filter_script:a":    "Could load arbitrary script files",
	"-protocol_whitelist": "Security risk - could enable dangerous protocols",
	"-protocol_blacklist": "Security risk - affects protocol handling",
	"-safe":               "Security setting should not be overridden",
	"-dump":               "Debugging flag that could expose sensitive info",
	"-hex":                "Debugging flag that could expose sensitive info",
}

// warnFlags are flags that trigger warnings but are allowed
var warnFlags = map[string]string{
	"-f":        "Output format is usually set via profile settings",
	"-c:v":      "Video codec is usually set via profile settings",
	"-c:a":      "Audio codec is usually set via profile settings",
	"-vcodec":   "Video codec is usually set via profile settings (use -c:v)",
	"-acodec":   "Audio codec is usually set via profile settings (use -c:a)",
	"-threads":  "Thread count is usually set via profile settings",
	"-re":       "Real-time mode can cause streaming issues with live sources",
	"-nostdin":  "Disables stdin which may affect signal handling",
	"-progress": "Progress output is managed by the relay system",
}

// ValidateCustomFlags validates custom FFmpeg flags for security and correctness
func ValidateCustomFlags(inputOptions, outputOptions, filterComplex string) FlagValidationResult {
	result := FlagValidationResult{
		Valid: true,
		Flags: []string{},
	}

	// Validate input options
	validateFlagString(&result, inputOptions, "input")

	// Validate output options
	validateFlagString(&result, outputOptions, "output")

	// Validate filter complex
	if filterComplex != "" {
		validateFilterComplex(&result, filterComplex)
	}

	return result
}

// containsDangerousPipe checks for single pipe characters (shell pipes) but not double pipes (logical OR)
// This is done as a separate function because Go's RE2 regex doesn't support negative lookahead
func containsDangerousPipe(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			// Check if it's a double pipe (||) - that's allowed in some FFmpeg filter contexts
			if i+1 < len(s) && s[i+1] == '|' {
				i++ // Skip the next pipe
				continue
			}
			// Check if previous char was also pipe
			if i > 0 && s[i-1] == '|' {
				continue
			}
			return true
		}
	}
	return false
}

// validateFlagString validates a string of FFmpeg flags
func validateFlagString(result *FlagValidationResult, flags string, context string) {
	if flags == "" {
		return
	}

	// Check for dangerous patterns
	for _, dp := range dangerousPatterns {
		if dp.pattern.MatchString(flags) {
			result.Valid = false
			result.Errors = append(result.Errors, "["+context+"] "+dp.message)
		}
	}

	// Check for pipe (shell injection) - done separately due to RE2 limitations
	if containsDangerousPipe(flags) {
		result.Valid = false
		result.Errors = append(result.Errors, "["+context+"] Pipe (|) detected")
	}

	// Check quote balancing
	if err := checkQuoteBalance(flags); err != "" {
		result.Valid = false
		result.Errors = append(result.Errors, "["+context+"] "+err)
	}

	// Parse flags and check for blocked/warned flags
	parsedFlags := parseFlags(flags)
	result.Flags = append(result.Flags, parsedFlags...)

	for _, flag := range parsedFlags {
		// Check blocked flags
		if reason, blocked := blockedFlags[flag]; blocked {
			result.Valid = false
			result.Errors = append(result.Errors, "["+context+"] Flag '"+flag+"' is blocked: "+reason)
		}

		// Check warning flags
		if reason, warned := warnFlags[flag]; warned {
			result.Warnings = append(result.Warnings, "["+context+"] Flag '"+flag+"': "+reason)
		}

		// Check if flag starts with dash
		if !strings.HasPrefix(flag, "-") && flag != "" {
			// This might be a value, not a flag - that's OK
			continue
		}
	}

	// Add suggestions for common issues
	addSuggestions(result, flags, context)
}

// validateFilterComplex validates a filter complex string
func validateFilterComplex(result *FlagValidationResult, filter string) {
	// Check for dangerous patterns in filter
	for _, dp := range dangerousPatterns {
		if dp.pattern.MatchString(filter) {
			result.Valid = false
			result.Errors = append(result.Errors, "[filter_complex] "+dp.message)
		}
	}

	// Check quote/bracket balancing
	if err := checkBracketBalance(filter); err != "" {
		result.Valid = false
		result.Errors = append(result.Errors, "[filter_complex] "+err)
	}

	// Check for file references in filters (potential security risk)
	filePatterns := []struct {
		pattern *regexp.Regexp
		message string
	}{
		{regexp.MustCompile(`movie=`), "External file reference via movie= filter"},
		{regexp.MustCompile(`amovie=`), "External file reference via amovie= filter"},
		{regexp.MustCompile(`sendcmd=`), "Command injection risk via sendcmd= filter"},
		{regexp.MustCompile(`zmq=`), "External control via zmq= filter"},
	}

	for _, fp := range filePatterns {
		if fp.pattern.MatchString(filter) {
			result.Warnings = append(result.Warnings, "[filter_complex] "+fp.message)
		}
	}
}

// checkQuoteBalance verifies that quotes are balanced
func checkQuoteBalance(s string) string {
	singleQuotes := 0
	doubleQuotes := 0
	escaped := false

	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' {
			singleQuotes++
		}
		if r == '"' {
			doubleQuotes++
		}
	}

	if singleQuotes%2 != 0 {
		return "Unbalanced single quotes"
	}
	if doubleQuotes%2 != 0 {
		return "Unbalanced double quotes"
	}
	return ""
}

// checkBracketBalance verifies that brackets and parentheses are balanced
func checkBracketBalance(s string) string {
	stack := []rune{}
	pairs := map[rune]rune{
		')': '(',
		']': '[',
		'}': '{',
	}
	openers := map[rune]bool{'(': true, '[': true, '{': true}

	inQuote := false
	quoteChar := rune(0)

	for _, r := range s {
		// Handle quotes
		if r == '"' || r == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if r == quoteChar {
				inQuote = false
			}
			continue
		}

		if inQuote {
			continue
		}

		if openers[r] {
			stack = append(stack, r)
		} else if opener, isCloser := pairs[r]; isCloser {
			if len(stack) == 0 || stack[len(stack)-1] != opener {
				return "Unbalanced brackets: unexpected '" + string(r) + "'"
			}
			stack = stack[:len(stack)-1]
		}
	}

	if len(stack) > 0 {
		return "Unbalanced brackets: unclosed '" + string(stack[len(stack)-1]) + "'"
	}

	return ""
}

// parseFlags splits a flag string into individual flags
func parseFlags(s string) []string {
	var flags []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			current.WriteRune(r)
			continue
		}

		if r == '"' || r == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if r == quoteChar {
				inQuote = false
			}
			current.WriteRune(r)
			continue
		}

		if unicode.IsSpace(r) && !inQuote {
			if current.Len() > 0 {
				flags = append(flags, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		flags = append(flags, current.String())
	}

	return flags
}

// addSuggestions adds helpful suggestions based on the flags
func addSuggestions(result *FlagValidationResult, flags string, context string) {
	lowerFlags := strings.ToLower(flags)

	// Suggest using profile settings instead of raw flags
	if strings.Contains(lowerFlags, "-b:v") || strings.Contains(lowerFlags, "-vb") {
		result.Suggestions = append(result.Suggestions,
			"Consider using the 'video_bitrate' profile setting instead of -b:v flag")
	}

	if strings.Contains(lowerFlags, "-b:a") || strings.Contains(lowerFlags, "-ab") {
		result.Suggestions = append(result.Suggestions,
			"Consider using the 'audio_bitrate' profile setting instead of -b:a flag")
	}

	if strings.Contains(lowerFlags, "-preset") {
		result.Suggestions = append(result.Suggestions,
			"Consider using the 'video_preset' profile setting instead of -preset flag")
	}

	// Suggest proper MPEG-TS settings
	if context == "output" && strings.Contains(lowerFlags, "mpegts") {
		if !strings.Contains(lowerFlags, "flush_packets") {
			result.Suggestions = append(result.Suggestions,
				"For MPEG-TS output, consider adding '-flush_packets 1' for lower latency")
		}
	}

	// Warn about -re flag
	if strings.Contains(lowerFlags, "-re") {
		result.Suggestions = append(result.Suggestions,
			"The -re flag can cause issues with live streams. Only use if you understand the implications")
	}
}

// ParseAndApplyInputOptions parses input options string and applies to builder
func ParseAndApplyInputOptions(builder *CommandBuilder, inputOptions string) (*CommandBuilder, error) {
	if inputOptions == "" {
		return builder, nil
	}

	flags := parseFlags(inputOptions)
	for _, flag := range flags {
		builder.InputArgs(flag)
	}

	return builder, nil
}

// ParseAndApplyOutputOptions parses output options string and applies to builder
func ParseAndApplyOutputOptions(builder *CommandBuilder, outputOptions string) (*CommandBuilder, error) {
	if outputOptions == "" {
		return builder, nil
	}

	flags := parseFlags(outputOptions)
	for _, flag := range flags {
		builder.OutputArgs(flag)
	}

	return builder, nil
}

// ParseAndApplyFilterComplex parses filter complex string and applies to builder
func ParseAndApplyFilterComplex(builder *CommandBuilder, filterComplex string) (*CommandBuilder, error) {
	if filterComplex == "" {
		return builder, nil
	}

	// Filter complex is a single string argument
	builder.OutputArgs("-filter_complex", filterComplex)

	return builder, nil
}

// SanitizeFlag removes potentially dangerous characters from a flag value
// This is a last-resort sanitization - validation should catch issues first
func SanitizeFlag(flag string) string {
	// Remove any shell metacharacters
	dangerous := []string{";", "|", "&", "`", "$", "(", ")", "{", "}", "<", ">", "\n", "\r"}
	result := flag
	for _, d := range dangerous {
		result = strings.ReplaceAll(result, d, "")
	}
	return result
}
