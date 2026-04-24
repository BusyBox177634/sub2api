package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirstNonEmptyString(t *testing.T) {
	got := firstNonEmptyString(nil, 123, "   ", "\n hello world \t", "fallback")

	require.Equal(t, "hello world", got)
}
