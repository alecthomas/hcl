package hcl

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/alecthomas/repr"
	"github.com/stretchr/testify/require"
)

type Number int

func (n *Number) UnmarshalJSON(b []byte) error {
	s, _ := strconv.Unquote(string(b))
	switch s {
	case "one":
		*n = 1
	case "two":
		*n = 2
	default:
		return fmt.Errorf("invalid number %s", b)
	}
	return nil
}

func TestUnmarshal(t *testing.T) {
	type strBlock struct {
		Str string `hcl:"str"`
	}
	type labelledBlock struct {
		Name string `hcl:"name,label"`
		Attr string `hcl:"attr"`
	}
	timestamp, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	require.NoError(t, err)
	tests := []struct {
		name  string
		hcl   string
		dest  interface{}
		fail  string
		fixup func(interface{}) // fixup unmarshalled structs
	}{
		{name: "Embed",
			hcl: `
				str = "foo"
				bar = "bar"
			`,
			dest: struct {
				strBlock
				Bar string `hcl:"bar"`
			}{
				strBlock: strBlock{"foo"},
				Bar:      "bar",
			},
		},
		{name: "MixedBlockAndAttribute",
			hcl: `
				name = "foo"
				name {}
			`,
			dest: struct{}{},
			fail: "2:5: 3:5: name cannot be both block and attribute",
		},
		{name: "DuplicateAttribute",
			hcl: `
				name = "foo"
				name = "foo"
			`,
			dest: struct {
				Name string `hcl:"name"`
			}{},
			fail: "2:5: duplicate field \"name\" at 3:5",
		},
		{name: "BlockForAttribute",
			hcl: `
				name {}
			`,
			dest: struct {
				Name string `hcl:"name"`
			}{},
			fail: "2:5: expected an attribute for \"name\" but got a block",
		},
		{name: "ScalarAttributes",
			hcl: `
				str = "string"
				float = 1.234
			`,
			dest: struct {
				Str   string  `hcl:"str"`
				Float float64 `hcl:"float"`
			}{
				Str:   "string",
				Float: 1.234,
			},
		},
		{name: "MapAttribute",
			hcl: `
				map = {a: 1, b: 2}
			`,
			dest: struct {
				Map map[string]int `hcl:"map"`
			}{
				Map: map[string]int{
					"a": 1,
					"b": 2,
				},
			},
		},
		{name: "ListAttribute",
			hcl: `
				list = [1, 2, 3]
			`,
			dest: struct {
				List []int `hcl:"list"`
			}{
				List: []int{1, 2, 3},
			},
		},
		{name: "AllAttributes",
			hcl: `
				str = "string"
				float = 1.234
				list = [1, 2, 3]
				map = {"a": "astr", b: "str"}
			`,
			dest: struct {
				Str   string            `hcl:"str"`
				Float float64           `hcl:"float"`
				List  []int             `hcl:"list"`
				Map   map[string]string `hcl:"map"`
			}{
				Str:   "string",
				Float: 1.234,
				List:  []int{1, 2, 3},
				Map:   map[string]string{"a": "astr", "b": "str"},
			},
		},
		{name: "BlockNoLabels",
			hcl: `
				block {
					str = "str"
				}
			`,
			dest: struct {
				Block strBlock `hcl:"block,block"`
			}{
				Block: strBlock{Str: "str"},
			},
		},
		{name: "BlockWithLabels",
			hcl: `
				block name {
					attr = "attr"
				}
			`,
			dest: struct {
				Block labelledBlock `hcl:"block,block"`
			}{
				Block: labelledBlock{
					Name: "name",
					Attr: "attr",
				},
			},
		},
		{name: "BlockMissingLabels",
			hcl: `
				block {
					attr = "attr"
				}
			`,
			dest: struct {
				Block labelledBlock `hcl:"block,block"`
			}{},
			fail: "2:5: missing label \"name\"",
		},
		{name: "TooManyLabels",
			hcl: `
				block "label0" "label1" {
					attr = "foo"
				}
			`,
			dest: struct {
				Block labelledBlock `hcl:"block,block"`
			}{},
			fail: "2:5: too many labels for block \"block\"",
		},
		{name: "SliceOfBlocks",
			hcl: `
				block "name" {
					attr = "one"
				}
				block "name" {
					attr = "two"
				}
			`,
			dest: struct {
				Blocks []labelledBlock `hcl:"block,block"`
			}{
				Blocks: []labelledBlock{
					{Name: "name", Attr: "one"},
					{Name: "name", Attr: "two"},
				},
			},
		},
		{name: "Duration",
			hcl: `
				duration = "5s"
			`,
			dest: struct {
				Duration time.Duration `hcl:"duration"`
			}{
				Duration: time.Second * 5,
			},
		},
		{name: "Time",
			hcl: `
				time = "2020-01-02T15:04:05Z"
			`,
			dest: struct {
				Time time.Time `hcl:"time"`
			}{
				Time: timestamp,
			},
		},
		{name: "TextUnmarshaler",
			hcl: `
				ip = "8.8.8.8"
			`,
			dest: struct {
				IP net.IP `hcl:"ip"`
			}{
				IP: net.IPv4(8, 8, 8, 8),
			},
		},
		{name: "JSONUnmarshaler",
			hcl: `
				number = "one"
			`,
			dest: struct {
				Number Number `hcl:"number"`
			}{
				Number: 1,
			},
		},
		{name: "PointerScalars",
			hcl: `
				ptr = "one"
			`,
			dest: struct {
				Ptr *string `hcl:"ptr"`
			}{Ptr: strp("one")},
		},
		{name: "PointerList",
			hcl: `
				list = [1, 2]
			`,
			dest: struct {
				List *[]int `hcl:"list"`
			}{
				List: intlistp(1, 2),
			},
		},
		{name: "BlockPointer",
			hcl: `
				block {
					str = "str"
				}
			`,
			dest: struct {
				Block *strBlock `hcl:"block,block"`
			}{
				Block: &strBlock{Str: "str"},
			},
		},
		{name: "BlockSliceOfPointers",
			hcl: `
				block {
					str = "foo"
				}
				block {
					str = "bar"
				}
			`,
			dest: struct {
				Block []*strBlock `hcl:"block,block"`
			}{
				Block: []*strBlock{{Str: "foo"}, {Str: "bar"}},
			},
		},
		{name: "Remain",
			hcl: `
name = "hello"
world = "world"
how = 1
are = true
`,
			dest: remainStruct{
				Name: "hello",
				Remain: []*Entry{
					attr("are", hbool(true)),
					attr("how", num(1)),
					attr("world", str("world")),
				},
			},
			fixup: func(i interface{}) {
				normaliseEntries(i.(*remainStruct).Remain)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rv := reflect.New(reflect.TypeOf(test.dest))
			actual := rv.Interface()
			err := Unmarshal([]byte(test.hcl), actual)
			if test.fail != "" {
				require.EqualError(t, err, test.fail)
			} else {
				require.NoError(t, err)
				if test.fixup != nil {
					test.fixup(actual)
				}
				require.Equal(t,
					repr.String(test.dest, repr.Indent("  ")),
					repr.String(rv.Elem().Interface(), repr.Indent("  ")))
			}
		})
	}
}

type remainStruct struct {
	Name   string   `hcl:"name"`
	Remain []*Entry `hcl:",remain"`
}

func intlistp(i ...int) *[]int { return &i }

func strp(s string) *string { return &s }

const complexHCLExample = `
aws {
  credentials-provider = "ROTATING_JSON"
}

server {
  acl {
    disable = true
    get "/**" {
      users = ["*"]
      capabilities = ["users_service_owners"]
    }

    grpc "/mycompany.service.UserService/UpgradeUser" {
      services = ["servicea", "serviceb"]
      users = ["*"]
      capabilities = ["users_service_owners"]
    }

    // ACL for MergeUser.
    grpc "/mycompany.service.UserService/MergeUser" {
      services = ["servicea", "serviceb"]
      users = ["*"]
      capabilities = ["users_service_owners"]
    }

    grpc "/mycompany.service.UserService/AuthenticateUser" {
      services = ["servicea", "rewardly"]
      users = ["*"]
      capabilities = ["users_service_owners"]
    }

    grpc "/**" {
      services = ["servicea"]
      users = ["*"]
      capabilities = ["users_service_owners"]
    }
  }
}
`

type AWS struct {
	CredentialsProvider string `hcl:"credentials-provider"`
}
type Rule struct {
	Target       string   `hcl:"target,label"`
	Services     []string `hcl:"services,optional"`
	Users        []string `hcl:"users,optional"`
	Capabilities []string `hcl:"capabilities,optional"`
}
type ACL struct {
	Disable bool   `hcl:"disable"`
	GET     []Rule `hcl:"get,block"`
	POST    []Rule `hcl:"post,block"`
	PUT     []Rule `hcl:"put,block"`
	DELETE  []Rule `hcl:"delete,block"`
	GRPC    []Rule `hcl:"grpc,block"`
}
type Server struct {
	ACL                         ACL     `hcl:"acl,block"`
	CACert                      string  `hcl:"ca-cert,optional"`
	KeyPair                     string  `hcl:"key-pair,optional"`
	CycleConnectionsProbability float64 `hcl:"cycle-connections-probability,optional"`
}
type Config struct {
	AWS    AWS    `hcl:"aws,block"`
	Server Server `hcl:"server,block"`
}

func TestUnmarshalComplex(t *testing.T) {
	config := Config{}
	err := Unmarshal([]byte(complexHCLExample), &config)
	require.NoError(t, err)
	expected := Config{
		AWS: AWS{
			CredentialsProvider: "ROTATING_JSON",
		},
		Server: Server{
			ACL: ACL{
				Disable: true,
				GET: []Rule{
					{
						Target:       "/**",
						Users:        []string{"*"},
						Capabilities: []string{"users_service_owners"},
					},
				},
				GRPC: []Rule{
					{
						Target:       "/mycompany.service.UserService/UpgradeUser",
						Users:        []string{"*"},
						Capabilities: []string{"users_service_owners"},
						Services:     []string{"servicea", "serviceb"},
					},
					{
						Target:       "/mycompany.service.UserService/MergeUser",
						Users:        []string{"*"},
						Capabilities: []string{"users_service_owners"},
						Services:     []string{"servicea", "serviceb"},
					},
					{
						Target:       "/mycompany.service.UserService/AuthenticateUser",
						Users:        []string{"*"},
						Capabilities: []string{"users_service_owners"},
						Services:     []string{"servicea", "rewardly"},
					},
					{
						Target:       "/**",
						Users:        []string{"*"},
						Capabilities: []string{"users_service_owners"},
						Services:     []string{"servicea"},
					},
				},
			},
		},
	}
	require.Equal(t, expected, config)
}

func TestUnmarshalPointers(t *testing.T) {
	b := struct {
		F *time.Time `hcl:"f"`
	}{}
	err := Unmarshal([]byte(`
f = "2017-07-07T00:00:00Z"
`), &b)
	require.NoError(t, err)
	require.NotNil(t, b.F)
	require.Equal(t, time.Date(2017, 7, 7, 0, 0, 0, 0, time.UTC), *b.F)
}
