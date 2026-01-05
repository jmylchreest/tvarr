package main

import (
	"fmt"
	"strings"
	"time"
)

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// truncateString truncates a string to maxLen, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// ValidateM3U checks if the M3U content is valid.
func ValidateM3U(content string) (channelCount int, err error) {
	if !strings.HasPrefix(content, "#EXTM3U") {
		return 0, fmt.Errorf("invalid M3U: missing #EXTM3U header")
	}

	// Count EXTINF entries (one per channel)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXTINF:") {
			channelCount++
		}
	}

	if channelCount == 0 {
		return 0, fmt.Errorf("invalid M3U: no channels found")
	}

	return channelCount, nil
}

// ValidateXMLTV checks if the XMLTV content is valid.
func ValidateXMLTV(content string) (channelCount, programCount int, err error) {
	if !strings.Contains(content, "<?xml") || !strings.Contains(content, "<tv") {
		return 0, 0, fmt.Errorf("invalid XMLTV: missing XML declaration or tv element")
	}

	// Count channel and programme elements
	channelCount = strings.Count(content, "<channel ")
	if channelCount == 0 {
		channelCount = strings.Count(content, "<channel>")
	}
	programCount = strings.Count(content, "<programme ")
	if programCount == 0 {
		programCount = strings.Count(content, "<programme>")
	}

	if channelCount == 0 {
		return 0, 0, fmt.Errorf("invalid XMLTV: no channels found")
	}

	return channelCount, programCount, nil
}

// extractChannelName extracts the channel name from an EXTINF line.
func extractChannelName(line string) string {
	// Name is after the last comma in EXTINF line
	commaIdx := strings.LastIndex(line, ",")
	if commaIdx == -1 {
		return "Unknown"
	}
	return strings.TrimSpace(line[commaIdx+1:])
}

// extractAttribute extracts an attribute value from an EXTINF line.
func extractAttribute(line, attr string) string {
	// Look for attr="value" or attr='value'
	patterns := []string{attr + `="`, attr + `='`}
	for _, pattern := range patterns {
		idx := strings.Index(strings.ToLower(line), strings.ToLower(pattern))
		if idx == -1 {
			continue
		}
		start := idx + len(pattern)
		quote := line[idx+len(pattern)-1]
		end := strings.IndexByte(line[start:], quote)
		if end == -1 {
			continue
		}
		return line[start : start+end]
	}
	return ""
}

// extractXMLAttribute extracts an attribute value from an XML element.
func extractXMLAttribute(xml, attr string) string {
	pattern := attr + `="`
	idx := strings.Index(xml, pattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(pattern)
	end := strings.IndexByte(xml[start:], '"')
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}

// extractXMLElement extracts the text content of an XML element.
func extractXMLElement(xml, element string) string {
	startTag := "<" + element
	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		return ""
	}
	// Find the closing > of the start tag
	closeStart := strings.Index(xml[startIdx:], ">")
	if closeStart == -1 {
		return ""
	}
	contentStart := startIdx + closeStart + 1
	endTag := "</" + element + ">"
	endIdx := strings.Index(xml[contentStart:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(xml[contentStart : contentStart+endIdx])
}
