package hcl

import (
	"reflect"
)

// Schema reflects a schema from a Go value.
//
// A schema is  itself HCL.
func Schema(v interface{}) (*AST, error) {
	ast, err := marshalToAST(v, true)
	if err != nil {
		return nil, err
	}
	ast.Schema = true
	return ast, nil
}

// MustSchema constructs a schema from a Go type, or panics.
func MustSchema(v interface{}) *AST {
	ast, err := Schema(v)
	if err != nil {
		panic(err)
	}
	return ast
}

var (
	strType  = "string"
	numType  = "number"
	boolType = "boolean"
)

func attrSchema(t reflect.Type) (*Value, error) {
	if t == durationType || t == timeType || typeImplements(t, textMarshalerInterface) || typeImplements(t, jsonMarshalerInterface) {
		return &Value{Type: &strType}, nil
	}
	switch t.Kind() {
	case reflect.String:
		return &Value{Type: &strType}, nil

	case reflect.Slice:
		el, err := attrSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		return &Value{List: []*Value{el}, HaveList: true}, nil

	case reflect.Map:
		el, err := attrSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		return &Value{Map: []*MapEntry{{Key: &Value{Type: &strType}, Value: el}}, HaveMap: true}, nil

	case reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Value{Type: &numType}, nil

	case reflect.Bool:
		return &Value{Type: &boolType}, nil

	default:
		panic(t.String())
	}
}

func sliceToBlockSchema(t reflect.Type, tag tag) (*Block, error) {
	block := &Block{
		Name:     tag.name,
		Comments: tag.comments(),
	}
	var err error
	block.Body, block.Labels, err = structToEntries(reflect.New(t.Elem()).Elem(), true)
	return block, err
}
