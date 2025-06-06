package hcl

import (
	"fmt"
	"net"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/repr"
)

type numberTest int

func (n *numberTest) UnmarshalJSON(b []byte) error {
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

type customLabelType string

func (c *customLabelType) UnmarshalText(text []byte) error {
	*c = customLabelType(text) + "-custom"
	return nil
}

type test struct {
	name    string
	hcl     string
	dest    interface{}
	fail    string
	fixup   func(interface{}) // fixup unmarshalled structs
	options []MarshalOption
}

func runTests(t *testing.T, tests []test) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Helper()
			rv := reflect.New(reflect.TypeOf(test.dest))
			actual := rv.Interface()
			err := Unmarshal([]byte(test.hcl), actual, test.options...)
			if test.fail != "" {
				assert.EqualError(t, err, test.fail)
			} else {
				assert.NoError(t, err)
				if test.fixup != nil {
					test.fixup(actual)
				}
				assert.Equal(t,
					repr.String(test.dest, repr.Indent("  ")),
					repr.String(rv.Elem().Interface(), repr.Indent("  ")))
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	type strBlock struct {
		Str string `hcl:"str"`
	}
	type labelledBlock struct {
		Name string `hcl:"name,label"`
		Attr string `hcl:"attr"`
	}
	type varArgLabelBlock struct {
		Path []string `hcl:"path,label"`
		Attr string   `hcl:"attr"`
	}
	type jsonStrBlock struct {
		Str string `json:"str"`
	}
	timestamp, err := time.Parse(time.RFC3339, "2020-01-02T15:04:05Z")
	assert.NoError(t, err)
	tests := []test{
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
		{name: "MapWithNonStringKeysAttribute",
			hcl: `
				map = {1: a, 2: b}
			`,
			dest: struct {
				Map map[int]string `hcl:"map"`
			}{
				Map: map[int]string{
					1: "a",
					2: "b",
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
		{name: "BlockWithVarArgLabels",
			hcl: `
				block multiple labels varargs {
					attr = "attr"
				}
			`,
			dest: struct {
				Block varArgLabelBlock `hcl:"block,block"`
			}{
				Block: varArgLabelBlock{
					Path: []string{"multiple", "labels", "varargs"},
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
			fail: "2:5: failed to unmarshal block: missing label \"name\"",
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
			fail: "2:5: failed to unmarshal block: too many labels for block \"block\"",
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
				NumberTest numberTest `hcl:"number"`
			}{
				NumberTest: 1,
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
		{name: "PointerScalarsNil",
			hcl: `
			`,
			dest: struct {
				Ptr *string `hcl:"ptr,optional"`
			}{Ptr: nil},
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
		{
			name: "UnmarshalTextMarshallerLabel",
			hcl:  `block label {}`,
			dest: struct {
				Block struct {
					Label customLabelType `hcl:"label,label"`
				} `hcl:"block,block"`
			}{
				Block: struct {
					Label customLabelType `hcl:"label,label"`
				}{Label: "label-custom"},
			},
			fail:    "",
			fixup:   nil,
			options: nil,
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
				Remain: []Entry{
					attr("are", hbool(true)),
					attr("how", num(1)),
					attr("world", str("world")),
				},
			},
			fixup: func(i interface{}) {
				normaliseEntries(i.(*remainStruct).Remain)
			},
		},
		{name: "Remain with blocks",
			hcl: `
name = "hello"
nested {
  name = "my"
}
nested {
  name = "your"
}
message1 = "wonderful"
message2 = "world"
`,
			dest: remainStruct{
				Name: "hello",
				Nested: []*remainNested{
					{
						Name: "my",
					},
					{
						Name: "your",
					},
				},
				Remain: []Entry{
					attr("message1", str("wonderful")),
					attr("message2", str("world")),
				},
			},
			fixup: func(i interface{}) {
				normaliseEntries(i.(*remainStruct).Remain)
			},
		},
		{name: "UnmarshallJSONTaggedStruct",
			hcl: `
                block {
                    str = "str"
                }
            `,
			dest: struct {
				Block jsonStrBlock `json:"block,omitempty"`
			}{
				Block: jsonStrBlock{Str: "str"},
			},
			options: []MarshalOption{InferHCLTags(true)},
		},
		{name: "Octal",
			hcl: `octal = 0700`,
			dest: struct {
				Octal int `hcl:"octal"`
			}{
				Octal: 0700,
			}},
	}
	runTests(t, tests)
}

type remainStruct struct {
	Name   string          `hcl:"name"`
	Nested []*remainNested `hcl:"nested,optional"`
	Remain []Entry         `hcl:",remain"`
}

type remainNested struct {
	Name string `hcl:"name"`
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
	assert.NoError(t, err)
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
	assert.Equal(t, expected, config)
}

func TestUnmarshalBlock(t *testing.T) {
	config := `
	get "/**" {
		users = ["alec"]
	}
	`
	hcl, err := ParseString(config)
	assert.NoError(t, err)
	rule := &Rule{}
	err = UnmarshalBlock(hcl.Entries[0].(*Block), rule)
	assert.NoError(t, err)
	assert.Equal(t, &Rule{
		Target: "/**",
		Users:  []string{"alec"},
	}, rule)
}

func TestUnmarshalPointers(t *testing.T) {
	b := struct {
		F *time.Time `hcl:"f"`
	}{}
	err := Unmarshal([]byte(`
f = "2017-07-07T00:00:00Z"
`), &b)
	assert.NoError(t, err)
	assert.NotZero(t, b.F)
	assert.Equal(t, time.Date(2017, 7, 7, 0, 0, 0, 0, time.UTC), *b.F)
}

func TestUnmarshalPointers2(t *testing.T) {
	b := struct {
		F *struct {
			G string `hcl:"g"`
		} `hcl:"f,block"`
	}{}
	err := Unmarshal([]byte(`
f {
	g = "str"
}
`), &b)
	assert.NoError(t, err)
	assert.Equal(t, "str", b.F.G)
	data, err := Marshal(&b)
	assert.NoError(t, err)
	assert.Equal(t, "f {\n  g = \"str\"\n}\n", string(data))
}

type defaultStruct struct {
	Name           string                      `hcl:"name"`
	DefaultString  string                      `hcl:"" default:"not empty"`
	DefaultInt     int                         `hcl:"" default:"3"`
	DefaultFloat   float32                     `hcl:"" default:"3.00"`
	DefaultBoolean bool                        `hcl:"" default:"true"`
	DefaultMap     map[string]int              `hcl:"" default:"a=2;b=4;c=6"`
	DefaultSlice   []int32                     `hcl:"" default:"4,5,6,7,8,9,10"`
	NestedStruct   nestedStructureWithDefaults `hcl:"nested,block"`
	// This field is here to provide test case where if you don't have the block. the block won't be created
	NestedStruct2 nestedStructureWithDefaults `hcl:"nested2,block"`
}

type nestedStructureWithDefaults struct {
	RequiredField string `hcl:"requiredField"`
	DefaultString string `hcl:"" default:"nested"`
}

func TestDefaultValueValidCases(t *testing.T) {
	hcl := `
name = "name"
nested {
  requiredField = "required"
}
`
	expected := defaultStruct{
		Name:           "name",
		DefaultString:  "not empty",
		DefaultInt:     3,
		DefaultFloat:   3.00,
		DefaultBoolean: true,
		DefaultMap: map[string]int{
			"a": 2,
			"b": 4,
			"c": 6,
		},
		DefaultSlice: []int32{4, 5, 6, 7, 8, 9, 10},
		NestedStruct: nestedStructureWithDefaults{
			RequiredField: "required",
			DefaultString: "nested",
		},
	}

	actual := defaultStruct{}

	err := Unmarshal([]byte(hcl), &actual)
	assert.NoError(t, err)
	assert.Equal(t, actual, expected)

}

func TestDefaultValueInvalidCases(t *testing.T) {
	hcl := `
name = "a"
`
	tests := []test{
		{
			name: "AllowExtra",
			hcl:  `foo = 10`,
			dest: struct {
				Bar string `hcl:"bar,optional"`
			}{},
			options: []MarshalOption{AllowExtra(true)},
		},
		{
			name: "WrongInt",
			hcl:  hcl,
			dest: struct {
				Name string `hcl:"name"`
				Int  int32  `hcl:"integer" default:"abc"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error converting "abc" to int`,
		},
		{
			name: "WrongFloat",
			hcl:  hcl,
			dest: struct {
				Name  string  `hcl:"name"`
				FLoat float32 `hcl:"integer" default:"abc"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error converting "abc" to float`,
		},
		{
			name: "WrongBool",
			hcl:  hcl,
			dest: struct {
				Name string `hcl:"name"`
				Bool bool   `hcl:"integer" default:"abc"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error converting "abc" to bool`,
		},
		{
			name: "WrongMap",
			hcl:  hcl,
			dest: struct {
				Name string           `hcl:"name"`
				Map  map[string]int32 `hcl:"integer" default:"abc"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error parsing map "abc" into pairs`,
		},
		{
			name: "WrongMapValue",
			hcl:  hcl,
			dest: struct {
				Name string           `hcl:"name"`
				Map  map[string]int32 `hcl:"integer" default:"key1=2;key2=test"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error parsing map "test" into value, error parsing default value: error converting "test" to int`,
		},
		{
			name: "WrongMapSeparator",
			hcl:  hcl,
			dest: struct {
				Name string           `hcl:"name"`
				Map  map[string]int32 `hcl:"integer" default:"key1=2,key2=test"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error parsing map "2,key2" into value, error parsing default value: error converting "2,key2" to int`,
		},
		{
			name: "WrongSlice",
			hcl:  hcl,
			dest: struct {
				Name string  `hcl:"name"`
				Map  []int32 `hcl:"integer" default:"a,b"`
			}{
				Name: "a",
			},
			fail: `error parsing default value: error applying "a" to list: error parsing default value: error converting "a" to int`,
		}}

	runTests(t, tests)
}

func TestEnumValidCases(t *testing.T) {
	tests := []test{
		{
			name: "Enum Valid Cases",
			hcl: `
name = "test"
str_val = "a"
int_val = 5
float_val = 2.11
`,
			dest: struct {
				Name     string  `hcl:"name"`
				StrVal   string  `hcl:"str_val" enum:"a,b,c"`
				IntVal   int64   `hcl:"int_val" enum:"2,5,8"`
				FloatVal float64 `hcl:"float_val" enum:"2.11,5.32,8.91"`
			}{
				Name:     "test",
				StrVal:   "a",
				IntVal:   5,
				FloatVal: 2.11,
			},
		},
	}

	runTests(t, tests)
}

func TestEnumInvalidCases(t *testing.T) {
	tests := []test{
		{
			name: "StringMismatch",
			hcl: `
		name = "test"
		`,
			dest: struct {
				Name string `hcl:"name" enum:"a,b,c"`
			}{
				Name: "test",
			},
			fail: `value "test" does not match anything within enum "a", "b", "c"`,
		},
		{
			name: "FloatMismatch",
			hcl: `
		val = 2.33
		`,
			dest: struct {
				Val float64 `hcl:"val" enum:"2.11,2.21,5.22"`
			}{
				Val: 2.33,
			},
			fail: `value 2.33 does not match anything within enum 2.11, 2.21, 5.22`,
		},
		{
			name: "IntMismatch",
			hcl: `
		val = 17
		`,
			dest: struct {
				Val int32 `hcl:"val" enum:"10,25,100"`
			}{
				Val: 17,
			},
			fail: `value 17 does not match anything within enum 10, 25, 100`,
		},
		{
			name: "StringDefaultValueConflicts",
			hcl:  ``,
			dest: struct {
				Str string `hcl:"str" enum:"a,b,c" default:"d"`
			}{
				Str: "d",
			},
			fail: `default value conflicts with enum: value "d" does not match anything within enum "a", "b", "c"`,
		},
		{
			name: "IntDefaultValueConflicts",
			hcl:  ``,
			dest: struct {
				Val int `hcl:"val" enum:"5,8,10" default:"9"`
			}{
				Val: 9,
			},
			fail: `default value conflicts with enum: value 9 does not match anything within enum 5, 8, 10`,
		},
		{
			name: "FloatDefaultValueConflicts",
			hcl:  ``,
			dest: struct {
				Val float64 `hcl:"val" enum:"5.2,8,10.9" default:"9.01"`
			}{
				Val: 9.01,
			},
			fail: `default value conflicts with enum: value 9.01 does not match anything within enum 5.2, 8, 10.9`,
		},
		{
			name: "IntEnumParseError",
			hcl:  ``,
			dest: struct {
				Val int32 `hcl:"val" enum:"5.2,8,10.9" default:"9"`
			}{
				Val: 9,
			},
			fail: `default value conflicts with enum: error parsing enum: error converting "5.2" to int`,
		},
		{
			name: "FloatEnumParseError",
			hcl:  ``,
			dest: struct {
				Val float64 `hcl:"val" enum:"5.2,8,a" default:"9.2"`
			}{
				Val: 9.2,
			},
			fail: `default value conflicts with enum: error parsing enum: error converting "a" to float`,
		},
		{
			name:    "AttrWithoutValue",
			hcl:     `attr`,
			options: []MarshalOption{BareBooleanAttributes(true)},
			dest: struct {
				Attr bool `hcl:"attr"`
			}{
				Attr: true,
			},
		},
		{
			name: "AttrWithoutValueNotEnabled",
			hcl:  `attr`,
			dest: struct {
				Attr bool `hcl:"attr"`
			}{
				Attr: true,
			},
			fail: "1:1: failed to unmarshal value: expected = after attribute",
		},
	}

	runTests(t, tests)
}

func TestUnmarshalJsonTaggedBlock(t *testing.T) {
	hcl := `
str = "test"

config {
  key = "inferHCLTags"
  value = "yes"
}

refs {
  name = "ref1"
}

refs {
  name = "ref2"
}

refs {
  name = "ref3"
}
`
	expected := jsonTaggedSchema{
		Str: "test",
		Config: keyValue{
			Key:   "inferHCLTags",
			Value: "yes",
		},
		Options: nil,
		Refs:    []objectRef{{"ref1"}, {"ref2"}, {"ref3"}},
	}
	var actual jsonTaggedSchema
	err := Unmarshal([]byte(hcl), &actual)
	assert.NoError(t, err)
	assert.Equal(t, actual, expected)
}

func TestOrder(t *testing.T) {
	type A struct {
		Pos Position `hcl:"-"`
	}
	type B struct {
		Pos Position `hcl:"-"`
	}
	type Blocks struct {
		A *A
		B *B
	}
	pos := func(b *Blocks) Position {
		if b.A != nil {
			return b.A.Pos
		}
		return b.B.Pos
	}
	type Main struct {
		A []*A `hcl:"a,block"`
		B []*B `hcl:"b,block"`
	}

	var actual Main
	err := Unmarshal([]byte(`
a {}
b {}
a {}
b {}
`), &actual)
	assert.NoError(t, err)
	blocks := []*Blocks{}
	for _, a := range actual.A {
		blocks = append(blocks, &Blocks{A: a})
	}
	for _, b := range actual.B {
		blocks = append(blocks, &Blocks{B: b})
	}
	sort.Slice(blocks, func(i, j int) bool {
		return pos(blocks[i]).Line < pos(blocks[j]).Line
	})
	expected := []*Blocks{
		{A: &A{Pos: Position{Offset: 1, Line: 2, Column: 1}}},
		{B: &B{Pos: Position{Offset: 6, Line: 3, Column: 1}}},
		{A: &A{Pos: Position{Offset: 11, Line: 4, Column: 1}}},
		{B: &B{Pos: Position{Offset: 16, Line: 5, Column: 1}}},
	}
	assert.Equal(t, expected, blocks)
}

type nonemptyInterface interface {
	Hello()
}

func TestUnmarshallInterfaces(t *testing.T) {
	tests := []test{
		{
			name: "BasicStr",
			hcl:  "f = \"hello\"",
			dest: struct {
				IfaceStr any `hcl:"f"`
			}{
				IfaceStr: "hello",
			},
		},
		{
			name: "HeterogeneousSimpleVals",
			hcl:  "a = 123\nb = true\nc = 1.2\n",
			dest: struct {
				A interface{} `hcl:"a"`
				B interface{} `hcl:"b"`
				C interface{} `hcl:"c"`
			}{
				A: 123.0,
				B: true,
				C: 1.2,
			},
		},
		{
			name: "AnyToMap",
			hcl:  `ifaceval = {a: "hello", b: "bye"}`,
			dest: struct {
				Ifaceval interface{} `hcl:"ifaceval"`
			}{
				Ifaceval: map[string]interface{}{
					"a": "hello",
					"b": "bye",
				},
			},
		},
		{
			name: "AnyToNestedMap",
			hcl:  `ifaceval = {a: "hello", b: {c: "inner"} }`,
			dest: struct {
				Ifaceval interface{} `hcl:"ifaceval"`
			}{
				Ifaceval: map[string]interface{}{
					"a": "hello",
					"b": map[string]interface{}{
						"c": "inner",
					},
				},
			},
		},
		{
			name: "SliceToInterface",
			hcl:  `ifaveval = ["a", "b", "c"]`,
			dest: struct {
				Ifaceval interface{} `hcl:"ifaveval"`
			}{
				Ifaceval: []interface{}{"a", "b", "c"},
			},
		},
		{
			name: "NonemptyInterfaceFails",
			hcl:  `ifaceval = {a: "hello", b: "bye"}`,
			dest: struct {
				Ifaceval nonemptyInterface `hcl:"ifaceval"`
			}{
				Ifaceval: nil,
			},
			fail: "1:12: failed to unmarshal value: invalid interface target: expected any/interface{} but got hcl.nonemptyInterface",
		},
		{
			name: "Block",
			hcl: `s = "asdf"
                  block {
                    hello = "test"
                  }`,
			dest: struct {
				S     string      `hcl:"s"`
				Block interface{} `hcl:"block,optional"`
			}{
				S: "asdf",
				Block: map[string]interface{}{
					"hello": "test",
				},
			},
			// blocks shouldn't implicitly work as they should to be unmarshalled into a struct
			fail: "2:19: expected an attribute for \"block\" but got a block",
		},
	}
	runTests(t, tests)
}
