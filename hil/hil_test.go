package hil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type Config struct {
	Version string `hcl:"version"`
}

const configSource = `
version = "version-${commit}"
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
	}
	require.Equal(t, expected, actual)
}
