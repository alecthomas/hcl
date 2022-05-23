package hcl

import (
	"testing"

	require "github.com/alecthomas/assert/v2"
)

func TestDedent(t *testing.T) {
	require.Equal(t, "", dedent(""))
	require.Equal(t, "\n  ", dedent("\n  "))
	require.Equal(t, "\n", dedent("  \n  "))
	require.Equal(t, "  \n", dedent("    \n  "))
}
