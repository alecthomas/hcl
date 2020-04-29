package hcl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalAST(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	require.NoError(t, err)
	data, err := MarshalAST(ast)
	require.NoError(t, err)
	require.Equal(t,
		strings.TrimSpace(complexHCLExample),
		strings.TrimSpace(string(data)))
}

func TestMarshal(t *testing.T) {
	config := Config{}
	err := Unmarshal([]byte(complexHCLExample), &config)
	require.NoError(t, err)
	data, err := Marshal(&config)
	require.NoError(t, err)

	// Normalise the HCL by removing comments.
	ast, err := ParseString(complexHCLExample)
	require.NoError(t, err)
	err = StripComments(ast)
	require.NoError(t, err)

	normalised, err := MarshalAST(ast)
	require.NoError(t, err)

	require.Equal(t,
		strings.TrimSpace(string(normalised)),
		strings.TrimSpace(string(data)))
}
