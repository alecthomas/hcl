package hcl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDedent(t *testing.T) {
	require.Equal(t, "", dedent(""))
	require.Equal(t, "\n  ", dedent("\n  "))
	require.Equal(t, "\n", dedent("  \n  "))
	require.Equal(t, "  \n", dedent("    \n  "))
}
