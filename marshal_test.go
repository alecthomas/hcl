package hcl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	require.NoError(t, err)
	data, err := MarshalAST(ast)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(complexHCLExample), strings.TrimSpace(string(data)))
}
