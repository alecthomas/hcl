package hcl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFind(t *testing.T) {
	input := `
		attr = "attr"
		block {}
		map = {
			key: "value"
		}
	`
	ast, err := ParseString(input)
	require.NoError(t, err)

	nodes := Find(ast, "attr")
	require.Len(t, nodes, 1)
	require.IsType(t, &Attribute{}, nodes[0])

	nodes = Find(ast, "attr", "key", "block")
	require.Len(t, nodes, 3)
	require.IsType(t, &Attribute{}, nodes[0])
	require.IsType(t, &Block{}, nodes[1])
	require.IsType(t, &MapEntry{}, nodes[2])
}
