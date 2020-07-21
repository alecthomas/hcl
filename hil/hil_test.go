package hil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type Block struct {
	Label string `hcl:"label,label"`
}

type Config struct {
	Version string            `hcl:"version"`
	Block   Block             `hcl:"block,block"`
	Map     map[string]string `hcl:"map"`
	List    []string          `hcl:"list"`
}

const configSource = `
version = "version-${commit}"

map = {
	commit: "${commit}",
	"${commit}": "commit",
}

list = ["${commit}", "commit"]

block "label-${commit}" {
}
`

func TestHILUnmarshal(t *testing.T) {
	actual := &Config{}
	err := Unmarshal([]byte(configSource), actual, map[string]interface{}{
		"commit": "43237b5e44e12c78bf478cba06dac1b88aec988c",
	})
	if err != nil {
		panic(err)
	}
	expected := &Config{
		Version: "version-43237b5e44e12c78bf478cba06dac1b88aec988c",
		Block:   Block{Label: "label-43237b5e44e12c78bf478cba06dac1b88aec988c"},
		Map: map[string]string{
			"commit": "43237b5e44e12c78bf478cba06dac1b88aec988c",
			"43237b5e44e12c78bf478cba06dac1b88aec988c": "commit",
		},
		List: []string{"43237b5e44e12c78bf478cba06dac1b88aec988c", "commit"},
	}
	require.Equal(t, expected, actual)
}
