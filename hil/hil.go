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
	hilVars, err := ToHILVars(vars)
	if err != nil {
		return err
	}
	err = InterpolateEvalConfig(hclAST, &hil.EvalConfig{
		GlobalScope: &hilast.BasicScope{VarMap: hilVars},
	})
	if err != nil {
		return err
	}
	return hcl.UnmarshalAST(hclAST, v)
}

// ToHILVars converts a Go map to hashicorp/hil variables.
//
// Values in "vars" must be of type "int", "string", "map[string]interface{}"
// or "[]interface{}".
func ToHILVars(vars map[string]interface{}) (map[string]hilast.Variable, error) {
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

		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeInt,
			}

		case float32, float64:
			hilVars[key] = hilast.Variable{
				Value: value,
				Type:  hilast.TypeFloat,
			}

		default:
			return nil, fmt.Errorf("unsupported variable type %T for %q", value, key)
		}
	}
	return hilVars, nil
}

// Interpolate all string values in "node" using the given hil.EvalConfig.
func InterpolateEvalConfig(node hcl.Node, config *hil.EvalConfig) error {
	// Interpolate into AST.
	return hcl.Visit(node, func(node hcl.Node, next func() error) error {
		switch node := node.(type) {
		case *hcl.Value:
			if node.Str == nil {
				return next()
			}
			str, err := evalStr(config, *node.Str)
			if err != nil {
				return participle.Errorf(node.Pos, "%s", err)
			}
			node.Str = &str

		case *hcl.Block:
			for i, label := range node.Labels {
				str, err := evalStr(config, label)
				if err != nil {
					return participle.Errorf(node.Pos, "%s", err)
				}
				node.Labels[i] = str
			}
		}
		return next()
	})
}

func evalStr(config *hil.EvalConfig, str string) (string, error) {
	hilNode, err := hil.Parse(str)
	if err != nil {
		return "", err
	}
	out, err := hil.Eval(hilNode, config)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", out.Value), nil
}
