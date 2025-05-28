package hcl

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestDedent(t *testing.T) {
	assert.Equal(t, "", dedent(""))
	assert.Equal(t, "\n  ", dedent("\n  "))
	assert.Equal(t, "\n", dedent("  \n  "))
	assert.Equal(t, "  \n", dedent("    \n  "))
}
