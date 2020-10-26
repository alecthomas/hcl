package hcl

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testSchema struct {
	Str   string         `hcl:"str" help:"A string field."`
	Num   int            `hcl:"num,optional"`
	Bool  bool           `hcl:"bool"`
	List  []string       `hcl:"list"`
	Map   map[string]int `hcl:"map" help:"A map."`
	Block struct {
		Name string  `hcl:"name,label"`
		Attr *string `hcl:"attr"`
	} `hcl:"block,block" help:"A block."`
	BlockSlice []struct {
		Label0 string `hcl:"label0,label"`
		Label1 string `hcl:"label1,label"`
		Attr   string `hcl:"attr"`
	} `hcl:"block_slice,block" help:"Repeated blocks."`
	DefaultStr string `hcl:"default_str" default:"def"`
	EnumStr    string `hcl:"enum_str" enum:"a,b,c"`
}

type keyValue struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

type objectRef struct {
	Name string `json:"name"`
}

type jsonTaggedSchema struct {
	Str     string      `json:"str"`
	Config  keyValue    `json:"config"`
	Options *keyValue   `json:"options,omitempty"`
	Refs    []objectRef `json:"refs,omitempty"`
}

const expectedSchema = `
// A string field.
str = string
num = number // (optional)
bool = boolean
list = [string]
// A map.
map = {
  string: number,
}

// A block.
block "name" {
  attr = string
}

// Repeated blocks.
block_slice "label0" "label1" { // (repeated)
  attr = string
}

default_str = string // (optional)
enum_str = string
`
const expectedJSONSchema = `
{
  "entries": [
    {
      "attribute": {
        "comments": [
          "A string field."
        ],
        "key": "str",
        "value": {
          "type": "string"
        }
      }
    },
    {
      "attribute": {
        "key": "num",
        "value": {
          "type": "number"
        },
        "optional": true
      }
    },
    {
      "attribute": {
        "key": "bool",
        "value": {
          "type": "boolean"
        }
      }
    },
    {
      "attribute": {
        "key": "list",
        "value": {
          "have_list": true,
          "list": [
            {
              "type": "string"
            }
          ]
        }
      }
    },
    {
      "attribute": {
        "comments": [
          "A map."
        ],
        "key": "map",
        "value": {
          "have_map": true,
          "map": [
            {
              "key": {
                "type": "string"
              },
              "value": {
                "type": "number"
              }
            }
          ]
        }
      }
    },
    {
      "block": {
        "comments": [
          "A block."
        ],
        "name": "block",
        "labels": [
          "name"
        ],
        "body": [
          {
            "attribute": {
              "key": "attr",
              "value": {
                "type": "string"
              }
            }
          }
        ]
      }
    },
    {
      "block": {
        "comments": [
          "Repeated blocks."
        ],
        "name": "block_slice",
        "labels": [
          "label0",
          "label1"
        ],
        "body": [
          {
            "attribute": {
              "key": "attr",
              "value": {
                "type": "string"
              }
            }
          }
        ],
        "repeated": true
      }
    },
    {
      "attribute": {
        "key": "default_str",
        "value": {
          "type": "string"
        },
        "default": {
          "str": "def"
        },
        "optional": true
      }
    },
    {
      "attribute": {
        "key": "enum_str",
        "value": {
          "type": "string"
        },
        "enum": [
          {
            "str": "a"
          },
          {
            "str": "b"
          },
          {
            "str": "c"
          }
        ]
      }
    }
  ],
  "schema": true
}
`

func TestSchema(t *testing.T) {
	schema, err := Schema(&testSchema{})
	require.NoError(t, err)
	data, err := MarshalAST(schema)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(expectedSchema), strings.TrimSpace(string(data)))
	data, err = json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(expectedJSONSchema), strings.TrimSpace(string(data)))
}

func TestBlockSchema(t *testing.T) {
	type Block struct {
		Label string `hcl:"label,label"`
		Attr  string `hcl:"attr"`
	}
	schema, err := BlockSchema("block", &Block{})
	require.NoError(t, err)
	data, err := MarshalAST(schema)
	require.NoError(t, err)
	require.Equal(t,
		strings.TrimSpace(`
block "label" {
  attr = string
}
`),
		strings.TrimSpace(string(data)))
}

func TestJsonTaggedSchema(t *testing.T) {
	var val interface{}
	val = &jsonTaggedSchema{
		Str:     "testSchema",
		Config:  keyValue{"key1", "val1"},
		Options: &keyValue{},
		Refs:    []objectRef{{"ref11"}, {"ref12"}, {"ref13"}},
	}
	schema, err := Schema(val, InferHCLTags(true))
	require.NoError(t, err)
	data, err := MarshalAST(schema)
	require.NoError(t, err)
	expectedSchema := `
str = string

config {
  key = string
  value = string // (optional)
}

options {
  key = string
  value = string // (optional)
}

refs { // (repeated)
  name = string
}
    `
	require.Equal(t, strings.TrimSpace(expectedSchema), strings.TrimSpace(string(data)))
}
