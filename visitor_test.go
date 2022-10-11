package hcl

import (
	"reflect"
	"testing"

	require "github.com/alecthomas/assert/v2"
	"github.com/alecthomas/repr"
)

func TestFind(t *testing.T) {
	input := `
		attr = "attr"
		block {}
		map = {
			"key": "value"
		}
	`
	ast, err := ParseString(input)
	require.NoError(t, err)

	nodes := Find(ast, "attr")
	require.Equal(t, len(nodes), 1)
	require.Equal(t, reflect.TypeOf(&Attribute{}), reflect.TypeOf(nodes[0]))

	nodes = Find(ast, "attr", "key", "block")
	require.Equal(t, 3, len(nodes), repr.String(nodes, repr.Indent("  ")))
	require.Equal(t, reflect.TypeOf(&Attribute{}), reflect.TypeOf(nodes[0]))
	require.Equal(t, reflect.TypeOf(&Block{}), reflect.TypeOf(nodes[1]))
	require.Equal(t, reflect.TypeOf(&MapEntry{}), reflect.TypeOf(nodes[2]))
}
