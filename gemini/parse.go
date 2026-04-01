package gemini

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

var lengthMarker = regexp.MustCompile(`(\d+)\n`)

// extractJSONFromResponse parses Google's batchexecute response format.
// The response is a series of length-prefixed JSON frames, sometimes
// preceded by )]}' anti-XSSI prefix.
func extractJSONFromResponse(text string) ([]json.RawMessage, error) {
	content := text
	if strings.HasPrefix(content, ")]}'") {
		content = content[4:]
	}
	content = strings.TrimSpace(content)

	// Try length-prefixed framing first
	frames := parseByFrame(content)
	if len(frames) > 0 {
		return frames, nil
	}

	// Fallback: try parsing as plain JSON array
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		return arr, nil
	}

	// Fallback: NDJSON
	var collected []json.RawMessage
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw json.RawMessage
		if json.Unmarshal([]byte(line), &raw) == nil {
			collected = append(collected, raw)
		}
	}
	if len(collected) > 0 {
		return collected, nil
	}

	return nil, nil
}

// parseByFrame implements Google's length-prefixed framing protocol.
// Length values are in UTF-16 code units (matching JavaScript String.length).
func parseByFrame(content string) []json.RawMessage {
	var frames []json.RawMessage
	pos := 0
	runes := []rune(content)
	total := len(runes)

	for pos < total {
		// Skip whitespace
		for pos < total && isSpace(runes[pos]) {
			pos++
		}
		if pos >= total {
			break
		}

		// Match length marker
		remaining := string(runes[pos:])
		loc := lengthMarker.FindStringIndex(remaining)
		if loc == nil || loc[0] != 0 {
			break
		}

		lenStr := remaining[:loc[1]-1] // exclude the \n
		length, err := strconv.Atoi(lenStr)
		if err != nil {
			break
		}

		// Content starts after the digits (length marker uses UTF-16 units)
		contentStart := pos + len([]rune(lenStr))
		charCount := utf16UnitsToChars(runes, contentStart, length)

		if contentStart+charCount > total {
			break // incomplete frame
		}

		chunk := strings.TrimSpace(string(runes[contentStart : contentStart+charCount]))
		pos = contentStart + charCount

		if chunk == "" {
			continue
		}

		// Try to parse as JSON array and flatten
		var arr []json.RawMessage
		if json.Unmarshal([]byte(chunk), &arr) == nil {
			frames = append(frames, arr...)
		} else {
			frames = append(frames, json.RawMessage(chunk))
		}
	}

	return frames
}

// utf16UnitsToChars counts how many runes correspond to the given number
// of UTF-16 code units, starting from startIdx in the rune slice.
func utf16UnitsToChars(runes []rune, startIdx, utf16Units int) int {
	count := 0
	units := 0
	for units < utf16Units && startIdx+count < len(runes) {
		r := runes[startIdx+count]
		u := 1
		if r > 0xFFFF {
			u = 2
		}
		// Also handle surrogate pairs in the encoding
		if utf16.IsSurrogate(r) {
			u = 2
		}
		if units+u > utf16Units {
			break
		}
		units += u
		count++
	}
	return count
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// getNestedValue safely traverses a nested JSON structure (arrays) by index path.
func getNestedValue(data any, path ...int) any {
	current := data
	for _, idx := range path {
		arr, ok := current.([]any)
		if !ok || idx < 0 || idx >= len(arr) {
			return nil
		}
		current = arr[idx]
	}
	return current
}

// getNestedString is a convenience wrapper that returns a string or "".
func getNestedString(data any, path ...int) string {
	v := getNestedValue(data, path...)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
