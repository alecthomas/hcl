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

func TestRoundTripEmptyList(t *testing.T) {
	type conf struct {
		List []string `hcl:"list"`
	}
	expected := hcl(attr("list", list()))
	actual := &conf{}
	err := UnmarshalAST(expected, actual)
	require.NoError(t, err)
	require.Equal(t, &conf{
		List: []string{},
	}, actual)
}

func TestRoundTripEmptyMap(t *testing.T) {
	type conf struct {
		Map map[string]string `hcl:"map"`
	}
	expected := hcl(attr("map", hmap()))
	actual := &conf{}
	err := UnmarshalAST(expected, actual)
	require.NoError(t, err)
	require.Equal(t, &conf{
		Map: map[string]string{},
	}, actual)
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
		options  []MarshalOption
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
		{name: "JsonTags",
			src: &struct {
				Block struct {
					Str string `json:"str"`
				} `json:"block"`
			}{
				Block: struct {
					Str string `json:"str"`
				}{Str: "val"},
			},
			expected: `
block {
  str = "val"
}
`,
			options: []MarshalOption{InferHCLTags(true)},
		},
		{
			name: "default values",
			src: &struct {
				Str string `hcl:"strVal" default:"str"`
				// where default is not empty value anymore
				StrSameDefault   string           `hcl:"strSameDefault" default:"str"`
				StrDiffDefault   string           `hcl:"strDiffDefault" default:"str"`
				Int              int64            `hcl:"intVal" default:"1"`
				IntSameDefault   int64            `hcl:"intSameDefault" default:"1"`
				IntDiffDefault   int64            `hcl:"intDiffDefault" default:"1"`
				Float            float64          `hcl:"floatVal" default:"2.33"`
				FloatSameDefault float64          `hcl:"floatSameDefault" default:"2.33"`
				FloatDiffDefault float64          `hcl:"floatDiffDefault" default:"2.33"`
				Slice            []string         `hcl:"sliceVal" default:"a,b,c"`
				SliceSameDefault []string         `hcl:"sliceSameDefault" default:"a,b,c"`
				SliceDiffDefault []string         `hcl:"sliceDiffDefault" default:"a,b,c"`
				Map              map[string]int32 `hcl:"mapVal" default:"a=4;b=5;c=6"`
				MapSameDefault   map[string]int32 `hcl:"mapSameDefault" default:"a=4;b=5;c=6"`
				MapDiffDefault   map[string]int32 `hcl:"mapDiffDefault" default:"a=4;b=5;c=6"`
			}{
				StrSameDefault:   "str",
				StrDiffDefault:   "diff",
				IntSameDefault:   1,
				IntDiffDefault:   2,
				FloatSameDefault: 2.33,
				FloatDiffDefault: 3.44,
				SliceSameDefault: []string{"a", "b", "c"},
				SliceDiffDefault: []string{"c", "d", "e"},
				MapSameDefault: map[string]int32{
					"a": 4,
					"c": 6,
					"b": 5,
				},
				MapDiffDefault: map[string]int32{
					"e": 7,
					"f": 8,
					"g": 9,
				},
			},
			expected: `
strVal = ""
strDiffDefault = "diff"
intVal = 0
intDiffDefault = 2
floatVal = 0
floatDiffDefault = 3.44
sliceVal = []
sliceDiffDefault = ["c", "d", "e"]
mapVal = {
}
mapDiffDefault = {
  "e": 7,
  "f": 8,
  "g": 9,
}
`,
		},
		{name: "JsonBlockSlice",
			src: &struct {
				Block struct {
					Str string `json:"str"`
				} `json:"block"`
			}{
				Block: struct {
					Str string `json:"str"`
				}{Str: "val"},
			},
			expected: `
block {
  str = "val"
}
`,
			options: []MarshalOption{InferHCLTags(true)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := Marshal(test.src, test.options...)
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(test.expected), strings.TrimSpace(string(data)))
		})
	}
}

type TestStruct struct {
	Val         string `hcl:"val"`
	DefaultVal  string `hcl:"default_val" default:"test"`
	DefaultVal2 int64  `hcl:"default_val_2" default:"60"`
}

func TestUnmarshalThenMarshal(t *testing.T) {
	hcl := `
val = "val"
default_val = "2"
`
	v := &TestStruct{}

	err := Unmarshal([]byte(hcl), v)
	require.NoError(t, err)

	require.Equal(t, v.Val, "val")
	require.Equal(t, v.DefaultVal, "2")
	require.Equal(t, v.DefaultVal2, int64(60))

	marshalled, err := Marshal(v)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(hcl), strings.TrimSpace(string(marshalled)))

}
