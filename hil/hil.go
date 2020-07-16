// Package hil supports interpolation variables into an
// alecthomas/hcl.AST and subsequently into a Go structure.
package hil

import (
	"fmt"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/participle"
	"github.com/hashicorp/hil"
	hilast "github.com/hashicorp/hil/ast"
)

// Unmarshal HCL into v, interpolating the variables in "vars".
//
// Values in "vars" must be of type "int", "string", "map[string]interface{}"
// or "[]interface{}".
func Unmarshal(data []byte, v interface{}, vars map[string]interface{}) error {
	hclAST, err := hcl.ParseBytes(data)
	if err != nil {
		return err
	}
	// Convert map into HIL variables.
	hilVars := map[string]hilast.Variable{}
	for key, value := range vars {
		switch value := value.(type) {
		case string:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeString,
			}

		case []interface{}:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeList,
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
	err = InterpolateHIL(hclAST, &hil.EvalConfig{
		GlobalScope: &hilast.BasicScope{VarMap: hilVars},
	})
	if err != nil {
		return err
	}
	return hcl.UnmarshalAST(hclAST, v)
}

// InterpolateHIL interpolates all string values in "node" using the given hil.EvalConfig.
func InterpolateHIL(node hcl.Node, config *hil.EvalConfig) error {
	// Interpolate into AST.
	return hcl.Visit(node, func(node hcl.Node, next func() error) error {
		value, ok := node.(*hcl.Value)
		if !ok || value.Str == nil {
			return next()
		}

		hilNode, err := hil.Parse(*value.Str)
		if err != nil {
			return participle.Errorf(value.Pos, "%s", err.Error())
		}
		out, err := hil.Eval(hilNode, config)
		if err != nil {
			return participle.Errorf(value.Pos, "%s", err.Error())
		}
		str := fmt.Sprintf("%v", out.Value)
		value.Str = &str
		return next()
	})
}
