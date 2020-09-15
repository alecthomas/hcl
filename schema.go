package hcl

import (
	"fmt"
	"reflect"
)

// Schema reflects a schema from a Go value.
//
// A schema is itself HCL.
func Schema(v interface{}) (*AST, error) {
	ast, err := marshalToAST(v, true)
	if err != nil {
		return nil, err
	}
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

// BlockSchema reflects a block schema for a Go struct.
func BlockSchema(name string, v interface{}) (*AST, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a pointer to a struct not %T", v)
	}
	block, err := valueToBlock(rv.Elem(), tag{name: name, block: true}, true)
	if err != nil {
		return nil, err
	}
	return &AST{
		Entries: []*Entry{{Block: block}},
		Schema:  true,
	}, nil
}

// MustBlockSchema reflects a block schema from a Go struct, panicking if an error occurs.
func MustBlockSchema(name string, v interface{}) *AST {
	ast, err := BlockSchema(name, v)
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

	case reflect.Struct:
		panic("struct " + t.String() + " used as attribute, is it missing a \"block\" tag?")

	case reflect.Ptr:
		return attrSchema(t.Elem())

	default:
		panic(fmt.Sprintf("unsupported attribute type %s during schema reflection", t))
	}
}

func sliceToBlockSchema(t reflect.Type, tag tag) (*Block, error) {
	block := &Block{
		Name:     tag.name,
		Comments: tag.comments(),
		Repeated: true,
	}
	var err error
	block.Body, block.Labels, err = structToEntries(reflect.New(t.Elem()).Elem(), true)
	return block, err
}
