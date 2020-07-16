// Package main shows how to use hashicorp/hil to interpolate variables
// into an alecthomas/hcl.AST and subsequently into a Go structure.
package main

import (
	"fmt"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/participle"
	"github.com/alecthomas/repr"
	"github.com/hashicorp/hil"
	hilast "github.com/hashicorp/hil/ast"
)

type Config struct {
	Version string `hcl:"version"`
}

const configSource = `
version = "version-${commit}"
`

func Unmarshal(data []byte, v interface{}, vars map[string]interface{}) error {
	hclAST, err := hcl.ParseBytes(data)
	if err != nil {
		return err
	}
	hilVars := map[string]hilast.Variable{}
	for key, value := range vars {
		switch value := value.(type) {
		case string:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeString,
			}

		case map[string]interface{}:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeMap,
			}

		case int:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeInt,
			}

		default:
			return fmt.Errorf("unsupported variable type %T for %q", value, key)
		}
	}
	// Interpolate into AST.
	err = hcl.Visit(hclAST, func(node hcl.Node, next func() error) error {
		value, ok := node.(*hcl.Value)
		if !ok || value.Str == nil {
			return next()
		}

		hilNode, err := hil.Parse(*value.Str)
		if err != nil {
			return participle.Errorf(value.Pos, "%s", err.Error())
		}
		out, err := hil.Eval(hilNode, &hil.EvalConfig{
			GlobalScope: &hilast.BasicScope{VarMap: hilVars},
		})
		if err != nil {
			return participle.Errorf(value.Pos, "%s", err.Error())
		}
		str := fmt.Sprintf("%v", out.Value)
		value.Str = &str
		return next()
	})
	if err != nil {
		return err
	}
	return hcl.UnmarshalAST(hclAST, v)
}

func main() {
	config := &Config{}
	err := Unmarshal([]byte(configSource), config, map[string]interface{}{
		"commit": "43237b5e44e12c78bf478cba06dac1b88aec988c",
	})
	if err != nil {
		panic(err)
	}
	repr.Println(config)
}
