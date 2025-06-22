package hcl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestMarshalASTComplex(t *testing.T) {
	ast, err := ParseString(complexHCLExample)
	assert.NoError(t, err)
	data, err := MarshalAST(ast)
	assert.NoError(t, err)
	assert.Equal(t,
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
	assert.NoError(t, err)
	assert.Equal(t, &conf{
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
	assert.NoError(t, err)
	assert.Equal(t, &conf{
		Map: map[string]string{},
	}, actual)
}

func TestMarshalComplex(t *testing.T) {
	config := Config{}
	err := Unmarshal([]byte(complexHCLExample), &config)
	assert.NoError(t, err)
	data, err := Marshal(&config)
	assert.NoError(t, err)

	// Normalise the HCL by removing comments.
	ast, err := ParseString(complexHCLExample)
	assert.NoError(t, err)
	err = StripComments(ast)
	assert.NoError(t, err)

	normalised, err := MarshalAST(ast)
	assert.NoError(t, err)

	assert.Equal(t,
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
	assert.NoError(t, err)
	type varArgLabelBlock struct {
		Path []string `hcl:"path,label"`
	}
	tests := []struct {
		name     string
		src      interface{}
		expected string
		options  []MarshalOption
	}{
		{name: "DurationSchema",
			src: &struct {
				Delay time.Duration `hcl:"delay,optional" default:"24h"`
			}{},
			expected: `
delay = string(optional default("24h"))
`,
			options: []MarshalOption{asSchema()},
		},
		{name: "DurationPtrSchema",
			src: &struct {
				Delay *time.Duration `hcl:"delay,optional" default:"24h"`
			}{},
			expected: `
delay = string(optional default("24h"))
`,
			options: []MarshalOption{asSchema()},
		},
		{name: "VarArgBlockLabels",
			src: &struct {
				Block varArgLabelBlock `hcl:"block,block"`
			}{
				Block: varArgLabelBlock{
					Path: []string{"multiple", "labels", "varargs"},
				},
			},
			expected: `
block multiple labels varargs {}
`,
		},
		{name: "LongVarArgBlockLabels",
			src: &struct {
				Block varArgLabelBlock `hcl:"block,block"`
			}{
				Block: varArgLabelBlock{
					Path: []string{"multiple", "labels", "var-args", "really", "really is", "really", "really", "long", "labels", "that", "are", "really", "long"},
				},
			},
			expected: `
block multiple labels var-args really "really is" really really long labels that are
      really long {}
`,
		},
		{name: "SingleReallyLongLabel",
			src: &struct {
				Block struct {
					Label string `hcl:"label,label"`
				} `hcl:"block,block"`
			}{
				Block: struct {
					Label string `hcl:"label,label"`
				}{
					Label: "single label that is really really really really long with text that is really long",
				},
			},
			expected: `
block "single label that is really really really really long with text that is really long" {}
`,
		},
		{name: "Heredocs",
			src: &struct {
				Nested struct {
					Str string `hcl:"str"`
				} `hcl:"nested,block"`
			}{
				Nested: struct {
					Str string `hcl:"str"`
				}{
					Str: "hello\nworld\nwhat's",
				},
			},
			expected: `
nested {
  str = <<-EOF
hello
world
what's
EOF
}
`,
			options: []MarshalOption{HereDocsForMultiLine(2)},
		},
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
				Map2 map[int]string    `hcl:"map2"`
				List []int             `hcl:"list"`
			}{
				Map:  map[string]string{"hello": "world", "waz": "foo"},
				Map2: map[int]string{5: "world"},
				List: []int{1, 2, 3},
			},
			expected: `
map = {
  "hello": "world",
  "waz": "foo",
}
map2 = {
  5: "world",
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
			name: "DefaultValues",
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
		{name: "DontMarshalHelpTag",
			src: &struct {
				Attr string `hcl:"attr" help:"An attribute."`
			}{
				Attr: "string",
			},
			expected: `
attr = "string"
			`,
		},
		{name: "MarshalHelpTagIfWithSchemaComments",
			src: &struct {
				Attr string `hcl:"attr" help:"An attribute."`
			}{
				Attr: "string",
			},
			expected: `// An attribute.
attr = "string"
			`,
			options: []MarshalOption{WithSchemaComments(true)},
		},
		{name: "MarshalHelpAndEnumDefaultTagsIfWithSchemaComments",
			src: &struct {
				Attr string `hcl:"attr" help:"An attribute." enum:"blue,green" default:"blue"`
			}{
				Attr: "string",
			},
			expected: `// An attribute.
// enum: blue,green
// default: blue
attr = "string"
			`,
			options: []MarshalOption{WithSchemaComments(true)},
		},
		{name: "DontMarshalOmittedBlock",
			src: &struct {
				HTML *struct {
					URL string `hcl:"url"`
				} `hcl:"html,block"`
			}{},
			expected: ``},
		{name: "MarshallStructViaInterface",
			src: &struct {
				IfaceBlock interface{} `hcl:"outer,block"`
			}{
				IfaceBlock: &struct {
					Attr string `hcl:"attr"`
				}{
					Attr: "some string",
				},
			},
			expected: `outer {
  attr = "some string"
}
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := Marshal(test.src, test.options...)
			assert.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(test.expected), strings.TrimSpace(string(data)))
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
	assert.NoError(t, err)

	assert.Equal(t, v.Val, "val")
	assert.Equal(t, v.DefaultVal, "2")
	assert.Equal(t, v.DefaultVal2, int64(60))

	marshalled, err := Marshal(v)
	assert.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(hcl), strings.TrimSpace(string(marshalled)))

}

func TestOptionalDefaultOmitted(t *testing.T) {
	type Embedded struct {
		Inner *string `hcl:"inner,optional" default:"inner"`
	}
	type Root struct {
		Outer string `hcl:"outer,optional"`
		Embedded
	}
	data, err := Marshal(&Root{})
	assert.NoError(t, err)
	assert.Equal(t, "", string(data))
}

func TestMarshalAST(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		ast      *AST
	}{
		{name: "BlockWithTrailingComments",
			expected: `block {
  attr = false

  // trailing comment
}
`,
			ast: hcl(&Block{
				Name:             "block",
				Body:             []Entry{attr("attr", hbool(false))},
				TrailingComments: []string{"trailing comment"},
			}),
		},
		{
			name: "BlockWithDetachedCommentsAheadOfIt",
			expected: `// detached comment 1

// detached comment 2 (independent of detached comment 1)

// attached comment (attached to following block)
block {}

// detached comment 3 (not attached to either the preceding or following block)

block {}

// detached comment 4 (not attached to either the preceding block or following comment)

// trailing AST comment (not attached to preceding block)
`,
			ast: &AST{
				Entries: []Entry{
					&Comment{Comments: []string{"detached comment 1"}},
					&Comment{Comments: []string{"detached comment 2 (independent of detached comment 1)"}},
					&Block{
						Name:     "block",
						Comments: []string{"attached comment (attached to following block)"},
					},
					&Comment{Comments: []string{"detached comment 3 (not attached to either the preceding or following block)"}},
					&Block{Name: "block"},
					&Comment{Comments: []string{"detached comment 4 (not attached to either the preceding block or following comment)"}},
				},
				TrailingComments: []string{"trailing AST comment (not attached to preceding block)"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normaliseAST(test.ast)
			b, err := MarshalAST(test.ast)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, string(b))
		})
	}
}
