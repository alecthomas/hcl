package hcl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarshalASTComplex(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	require.NoError(t, err)
	data, err := MarshalAST(ast)
	require.NoError(t, err)
	require.Equal(t,
		strings.TrimSpace(complexHCLExample),
		strings.TrimSpace(string(data)))
}

func TestMarshalComplex(t *testing.T) {
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

type textMarshalValue struct{ text string }

func (t *textMarshalValue) MarshalText() (text []byte, err error) { return []byte(t.text), nil }

type jsonMarshalValue struct{ text string }

func (j *jsonMarshalValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"hello": j.text})
}

func TestMarshal(t *testing.T) {
	timestamp, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err)
	tests := []struct {
		name     string
		src      interface{}
		expected string
	}{
		{name: "Scalars",
			src: &struct {
				Str   string  `hcl:"str"`
				Int   int     `hcl:"int"`
				Float float64 `hcl:"float"`
				Bool  bool    `hcl:"bool"`
			}{
				Str:   "str",
				Int:   123,
				Float: 123.456,
				Bool:  true,
			},
			expected: `
str = "str"
int = 123
float = 123.456
bool = true
`,
		},
		{name: "ListsAndMaps",
			src: &struct {
				Map  map[string]string `hcl:"map"`
				List []int             `hcl:"list"`
			}{
				Map:  map[string]string{"hello": "world", "waz": "foo"},
				List: []int{1, 2, 3},
			},
			expected: `
map = {
  "hello": "world",
  "waz": "foo",
}
list = [1, 2, 3]
`,
		},
		{name: "DurationAndTime",
			src: &struct {
				Time     time.Time     `hcl:"time"`
				Duration time.Duration `hcl:"duration"`
			}{
				Time:     timestamp,
				Duration: time.Second * 5,
			},
			expected: `
time = "2020-01-02T15:04:05Z"
duration = "5s"
`,
		},
		{name: "Marshalers",
			src: &struct {
				Text textMarshalValue `hcl:"text"`
				JSON jsonMarshalValue `hcl:"json"`
			}{
				Text: textMarshalValue{"hello"},
				JSON: jsonMarshalValue{"world"},
			},
			expected: `
text = "hello"
json = "{\"hello\":\"world\"}"
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := Marshal(test.src)
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(test.expected), strings.TrimSpace(string(data)))
		})
	}
}
