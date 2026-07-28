package tool_test

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github/musuyin/agent-weave/internal/tool"
)

func TestTruncate_NoOp(t *testing.T) {
	s := "hello"
	assert.Equal(t, s, tool.Truncate(s, 10))
}

func TestTruncate_ExactLimit(t *testing.T) {
	s := "hello"
	assert.Equal(t, s, tool.Truncate(s, 5))
}

func TestTruncate_ASCII(t *testing.T) {
	s := "hello world"
	got := tool.Truncate(s, 5)
	assert.Equal(t, "hello", got)
}

func TestTruncate_UTF8Boundary(t *testing.T) {
	// "日" is 3 bytes in UTF-8. Truncating at byte 4 must not cut inside the second rune.
	s := "日本語"
	got := tool.Truncate(s, 4)
	assert.True(t, utf8.ValidString(got), "result must be valid UTF-8")
	// Should land on the 3-byte boundary of the first rune.
	assert.Equal(t, "日", got)
}

func TestTruncate_MaxToolResultBytes(t *testing.T) {
	// Build a string just over the cap and verify it is truncated.
	big := make([]byte, tool.MaxToolResultBytes+100)
	for i := range big {
		big[i] = 'a'
	}
	got := tool.Truncate(string(big), tool.MaxToolResultBytes)
	assert.Len(t, got, tool.MaxToolResultBytes)
}
