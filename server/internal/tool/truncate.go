package tool

// MaxToolResultBytes is the hard cap on tool result size sent back to the model.
const MaxToolResultBytes = 16 * 1024

// Truncate returns s truncated to at most maxBytes, cutting at a valid UTF-8 rune boundary.
// If len(s) <= maxBytes, s is returned unchanged.
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Scan back from maxBytes to find a valid UTF-8 rune start byte.
	i := maxBytes
	for i > 0 && isUTF8Continuation(s[i]) {
		i--
	}
	return s[:i]
}

// isUTF8Continuation reports whether b is a UTF-8 continuation byte (10xxxxxx).
func isUTF8Continuation(b byte) bool {
	return b&0xC0 == 0x80
}
