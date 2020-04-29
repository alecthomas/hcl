package hcl

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// Marshal a Go type to HCL.
func Marshal(v interface{}) ([]byte, error) {
	ast, err := MarshalToAST(v)
	if err != nil {
		return nil, err
	}
	return MarshalAST(ast)
}

// MarshalToAST marshals a Go type to a hcl.AST.
func MarshalToAST(v interface{}) (*AST, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("expected a pointer to a struct, not %T", v)
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a pointer to a struct, not %T", v)
	}
	var (
		err    error
		labels []string
		ast    = &AST{}
	)
	ast.Entries, labels, err = structToEntries(rv)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		return nil, fmt.Errorf("unexpected labels %s at top level", strings.Join(labels, ", "))
	}
	return ast, nil
}

// MarshalAST marshals an AST to HCL bytes.
func MarshalAST(ast *AST) ([]byte, error) {
	w := &bytes.Buffer{}
	err := MarshalASTToWriter(ast, w)
	return w.Bytes(), err
}

// MarshalASTToWriter marshals a hcl.AST to an io.Writer.
func MarshalASTToWriter(ast *AST, w io.Writer) error {
	return marshalEntries(w, "", ast.Entries)
}

func structToEntries(v reflect.Value) (entries []*Entry, labels []string, err error) {
	fields, err := flattenFields(v)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range fields {
		tag := parseTag(v.Type(), field.t)
		if tag.label { // nolint: gocritic
			labels = append(labels, field.v.String())
		} else if tag.block {
			if field.v.Kind() == reflect.Slice {
				blocks, err := sliceToBlocks(field.v, tag)
				if err != nil {
					return nil, nil, err
				}
				for _, block := range blocks {
					entries = append(entries, &Entry{Block: block})
				}
			} else {
				block, err := valueToBlock(field.v, tag)
				if err != nil {
					return nil, nil, err
				}
				entries = append(entries, &Entry{Block: block})
			}
		} else {
			if tag.optional && field.v.IsZero() {
				continue
			}
			attr, err := fieldToAttr(field, tag)
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, &Entry{Attribute: attr})
		}
	}
	return entries, labels, nil
}

func fieldToAttr(field field, tag tag) (*Attribute, error) {
	attr := &Attribute{
		Key: tag.name,
	}
	var err error
	attr.Value, err = valueToValue(field.v)
	return attr, err
}

func valueToValue(v reflect.Value) (*Value, error) {
	switch v.Kind() {
	case reflect.String:
		s := v.Interface().(string)
		return &Value{Str: &s}, nil

	case reflect.Slice:
		list := []*Value{}
		for i := 0; i < v.Len(); i++ {
			el := v.Index(i)
			elv, err := valueToValue(el)
			if err != nil {
				return nil, err
			}
			list = append(list, elv)
		}
		return &Value{List: list}, nil

	case reflect.Map:
		entries := []*MapEntry{}
		for _, key := range v.MapKeys() {
			value, err := valueToValue(v.MapIndex(key))
			if err != nil {
				return nil, err
			}
			entries = append(entries, &MapEntry{
				Key:   key.String(),
				Value: value,
			})
		}
		return &Value{Map: entries}, nil

	case reflect.Float32, reflect.Float64:
		return &Value{Number: big.NewFloat(v.Float())}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Value{Number: big.NewFloat(0).SetInt64(v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Value{Number: big.NewFloat(0).SetUint64(v.Uint())}, nil

	default:
		panic(v.Type().String())
	}
}

func valueToBlock(v reflect.Value, tag tag) (*Block, error) {
	block := &Block{
		Name: tag.name,
	}
	var err error
	block.Body, block.Labels, err = structToEntries(v)
	return block, err
}

func sliceToBlocks(sv reflect.Value, tag tag) ([]*Block, error) {
	blocks := []*Block{}
	for i := 0; i != sv.Len(); i++ {
		block, err := valueToBlock(sv.Index(i), tag)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func marshalEntries(w io.Writer, indent string, entries []*Entry) error {
	for i, entry := range entries {
		marshalComments(w, indent, entry.Comments)
		if entry.Block != nil { // nolint: gocritic
			if err := marshalBlock(w, indent, entry.Block); err != nil {
				return err
			}
			if i != len(entries)-1 {
				fmt.Fprintln(w)
			}
		} else if entry.Attribute != nil {
			if err := marshalAttribute(w, indent, entry.Attribute); err != nil {
				return err
			}
		} else {
			panic("??")
		}
	}
	return nil
}

func marshalAttribute(w io.Writer, indent string, attribute *Attribute) error {
	fmt.Fprintf(w, "%s%s = ", indent, attribute.Key)
	err := marshalValue(w, indent, attribute.Value)
	if err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

func marshalValue(w io.Writer, indent string, value *Value) error {
	if value.Map != nil {
		return marshalMap(w, indent+"  ", value.Map)
	}
	fmt.Fprintf(w, "%s", value)
	return nil
}

func marshalMap(w io.Writer, indent string, entries []*MapEntry) error {
	fmt.Fprintln(w, "{")
	for _, entry := range entries {
		marshalComments(w, indent, entry.Comments)
		fmt.Fprintf(w, "%q: ", entry.Key)
		if err := marshalValue(w, indent+"  ", entry.Value); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func marshalBlock(w io.Writer, indent string, block *Block) error {
	fmt.Fprintf(w, "%s%s ", indent, block.Name)
	for _, label := range block.Labels {
		fmt.Fprintf(w, "%q ", label)
	}
	fmt.Fprintln(w, "{")
	err := marshalEntries(w, indent+"  ", block.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func marshalComments(w io.Writer, indent string, comments []string) {
	for _, comment := range comments {
		fmt.Fprintf(w, "%s%s\n", indent, comment)
	}
}
