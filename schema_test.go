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
		Name string `hcl:"name,label"`
		Attr string `hcl:"attr"`
	} `hcl:"block,block" help:"A block."`
	BlockSlice []struct {
		Label0 string `hcl:"label0,label"`
		Label1 string `hcl:"label1,label"`
		Attr   string `hcl:"attr"`
	} `hcl:"block_slice,block" help:"Repeated blocks."`
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
