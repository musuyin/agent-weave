package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/model/repository"
)

func TestContentBlocks_RoundTrip(t *testing.T) {
	original := repository.ContentBlocks{
		{Type: "text", Text: "hello world"},
		{Type: "text", Text: "second block"},
	}

	val, err := original.Value()
	require.NoError(t, err)

	var restored repository.ContentBlocks
	require.NoError(t, restored.Scan(val))
	assert.Equal(t, original, restored)
}

func TestContentBlocks_ScanBytes(t *testing.T) {
	raw := []byte(`[{"type":"text","text":"hi"}]`)
	var cb repository.ContentBlocks
	require.NoError(t, cb.Scan(raw))
	assert.Equal(t, repository.ContentBlocks{{Type: "text", Text: "hi"}}, cb)
}

func TestContentBlocks_ScanString(t *testing.T) {
	raw := `[{"type":"text","text":"hi"}]`
	var cb repository.ContentBlocks
	require.NoError(t, cb.Scan(raw))
	assert.Equal(t, repository.ContentBlocks{{Type: "text", Text: "hi"}}, cb)
}

func TestContentBlocks_ScanUnsupportedType(t *testing.T) {
	var cb repository.ContentBlocks
	assert.Error(t, cb.Scan(42))
}

func TestContentBlocks_Empty(t *testing.T) {
	empty := repository.ContentBlocks{}
	val, err := empty.Value()
	require.NoError(t, err)

	var restored repository.ContentBlocks
	require.NoError(t, restored.Scan(val))
	assert.Equal(t, empty, restored)
}

func TestStringSlice_RoundTrip(t *testing.T) {
	original := repository.StringSlice{"id-1", "id-2", "id-3"}

	val, err := original.Value()
	require.NoError(t, err)

	var restored repository.StringSlice
	require.NoError(t, restored.Scan(val))
	assert.Equal(t, original, restored)
}

func TestStringSlice_Empty(t *testing.T) {
	empty := repository.StringSlice{}
	val, err := empty.Value()
	require.NoError(t, err)

	var restored repository.StringSlice
	require.NoError(t, restored.Scan(val))
	assert.Equal(t, empty, restored)
}

func TestStringSlice_ScanUnsupportedType(t *testing.T) {
	var s repository.StringSlice
	assert.Error(t, s.Scan(42))
}
