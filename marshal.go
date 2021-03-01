package hcl

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/participle/lexer"
)

// marshalOptions defines options for the marshalling/unmarshalling process
type marshalOptions struct {
	inferHCLTags         bool
	hereDocsForMultiline int
}

// MarshalOption configures optional marshalling behaviour.
type MarshalOption func(options *marshalOptions)

// InferHCLTags specifies whether to infer behaviour if hcl:"" tags are not present.
//
// This currently just means that all structs become blocks.
func InferHCLTags(v bool) MarshalOption {
	return func(options *marshalOptions) {
		options.inferHCLTags = v
	}
}

// HereDocsForMultiLine will marshal multi-line strings >= n lines as indented
// heredocs rather than quoted strings.
func HereDocsForMultiLine(n int) MarshalOption {
	return func(options *marshalOptions) {
		options.hereDocsForMultiline = n
	}
}

// newMarshalOptions creates marshal options from a set of options
func newMarshalOptions(options ...MarshalOption) *marshalOptions {
	opt := &marshalOptions{}
	for _, option := range options {
		option(opt)
	}
	return opt
}

// Marshal a Go type to HCL.
func Marshal(v interface{}, options ...MarshalOption) ([]byte, error) {
	ast, err := MarshalToAST(v, options...)
	if err != nil {
		return nil, err
	}
	return MarshalAST(ast)
}

// MarshalToAST marshals a Go type to a hcl.AST.
func MarshalToAST(v interface{}, options ...MarshalOption) (*AST, error) {
	return marshalToAST(v, false, newMarshalOptions(options...))
}

// MarshalAST marshals an AST to HCL bytes.
func MarshalAST(ast Node) ([]byte, error) {
	w := &bytes.Buffer{}
	err := MarshalASTToWriter(ast, w)
	return w.Bytes(), err
}

// MarshalASTToWriter marshals a hcl.AST to an io.Writer.
func MarshalASTToWriter(ast Node, w io.Writer) error {
	return marshalNode(w, "", ast)
}

func marshalToAST(v interface{}, schema bool, opt *marshalOptions) (*AST, error) {
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
		ast    = &AST{
			Schema: schema,
		}
	)
	ast.Entries, labels, err = structToEntries(rv, schema, opt)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		return nil, fmt.Errorf("unexpected labels %s at top level", strings.Join(labels, ", "))
	}
	return ast, nil
}

func structToEntries(v reflect.Value, schema bool, opt *marshalOptions) (entries []*Entry, labels []string, err error) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !schema {
				return nil, nil, nil
			}
			v = reflect.New(v.Type().Elem())
		}
		v = v.Elem()
	}
	fields, err := flattenFields(v)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range fields {
		tag := parseTag(v.Type(), field, opt)
		switch {
		case tag.label:
			if schema {
				labels = append(labels, tag.name)
			} else {
				labels = append(labels, field.v.String())
			}

		case tag.block:
			if field.v.Kind() == reflect.Slice {
				var blocks []*Block
				if schema {
					block, err := sliceToBlockSchema(field.v.Type(), tag, opt)
					if err == nil {
						block.Repeated = true
						blocks = append(blocks, block)
					}
				} else {
					blocks, err = sliceToBlocks(field.v, tag, opt)
				}
				if err != nil {
					return nil, nil, err
				}
				for _, block := range blocks {
					entries = append(entries, &Entry{Block: block})
				}
			} else {
				block, err := valueToBlock(field.v, tag, schema, opt)
				if err != nil {
					return nil, nil, err
				}
				entries = append(entries, &Entry{Block: block})
			}

		default:
			attr, err := fieldToAttr(field, tag, schema, opt)
			if err != nil {
				return nil, nil, err
			}
			hasDefaultAndEqualsValue := attr.Default != nil && attr.Value.String() == attr.Default.String()
			noDefaultButIsZero := attr.Default == nil && field.v.IsZero()
			valueEqualsDefault := noDefaultButIsZero || hasDefaultAndEqualsValue
			if tag.optional && !schema && valueEqualsDefault {
				continue
			}
			entries = append(entries, &Entry{Attribute: attr})
		}
	}
	return entries, labels, nil
}

func fieldToAttr(field field, tag tag, schema bool, opt *marshalOptions) (*Attribute, error) {
	attr := &Attribute{
		Key:      tag.name,
		Comments: tag.comments(),
	}
	var err error
	if schema {
		attr.Value, err = attrSchema(field.v.Type())
	} else {
		attr.Value, err = valueToValue(field.v, opt)
	}
	if err != nil {
		return nil, err
	}
	attr.Default, err = defaultValueFromTag(field, tag.defaultValue)
	if err != nil {
		return nil, err
	}
	attr.Optional = (tag.optional || attr.Default != nil) && schema
	attr.Enum, err = enumValuesFromTag(field, tag.enum)
	return attr, err
}

func defaultValueFromTag(f field, defaultValue string) (*Value, error) {
	v, err := valueFromTag(f, defaultValue)
	if err != nil {
		return nil, fmt.Errorf("error parsing default value: %v", err)
	}

	return v, nil
}

// enumValuesFromTag parses the enum string from tag into a list of Values
func enumValuesFromTag(f field, enum string) ([]*Value, error) {
	if enum == "" {
		return nil, nil
	}

	enums := strings.Split(enum, ",")
	list := make([]*Value, 0, len(enums))
	for _, e := range enums {
		enumVal, err := valueFromTag(f, e)
		if err != nil {
			return nil, fmt.Errorf("error parsing enum: %v", err)
		}

		list = append(list, enumVal)
	}

	return list, nil

}

func valueFromTag(f field, defaultValue string) (*Value, error) {
	if defaultValue == "" {
		return nil, nil
	}

	k := f.v.Kind()
	if k == reflect.Ptr {
		k = f.v.Elem().Kind()
	}

	switch k {
	case reflect.String:
		return &Value{Str: &defaultValue}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(defaultValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to int", defaultValue)
		}
		return &Value{
			Number: big.NewFloat(0).SetInt64(n),
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(defaultValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to uint", defaultValue)
		}
		return &Value{
			Number: big.NewFloat(0).SetUint64(n),
		}, nil
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(defaultValue, 10)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to float", defaultValue)
		}
		return &Value{
			Number: big.NewFloat(n),
		}, nil
	case reflect.Bool:
		b, err := strconv.ParseBool(defaultValue)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to bool", defaultValue)
		}
		v := Bool(b)
		return &Value{
			Bool: &v,
		}, nil
	case reflect.Map:
		mapEntries := []*MapEntry{}
		entries := strings.Split(defaultValue, ";")
		mEntries := make(map[string]*MapEntry)
		keys := make([]string, 0, len(entries))
		for _, entry := range entries {
			pair := strings.Split(entry, "=")
			if len(pair) < 2 {
				return nil, fmt.Errorf("error parsing map %q into pairs", entry)
			}
			v := pair[1]
			key := &Value{Str: &pair[0]}
			valueType := f.t.Type.Elem()
			valueKind := valueType.Kind()
			if valueKind == reflect.Map || valueKind == reflect.Slice {
				return nil, fmt.Errorf("nested structures are not supported in map")
			}
			valueField := field{
				t: reflect.StructField{},
				v: reflect.New(valueType),
			}
			val, err := defaultValueFromTag(valueField, v)
			if err != nil {
				return nil, fmt.Errorf("error parsing map %q into value, %v", v, err)
			}
			// so that we deduplicate the keys, last one up
			mEntries[pair[0]] = &MapEntry{
				Pos:   lexer.Position{},
				Key:   key,
				Value: val,
			}
		}

		for k := range mEntries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			mapEntries = append(mapEntries, mEntries[k])
		}

		return &Value{
			HaveMap: true,
			Map:     mapEntries,
		}, nil
	case reflect.Slice:
		slice := []*Value{}
		list := strings.Split(defaultValue, ",")
		valueType := f.t.Type.Elem()
		valueKind := valueType.Kind()
		if valueKind == reflect.Map || valueKind == reflect.Slice {
			return nil, fmt.Errorf("nested map or slice is not supported in slice")
		}
		valueField := field{
			t: reflect.StructField{},
			v: reflect.New(valueType),
		}
		for _, item := range list {
			value, err := defaultValueFromTag(valueField, item)
			if err != nil {
				return nil, fmt.Errorf("error applying %q to list: %v", item, err)
			}
			slice = append(slice, value)
		}

		return &Value{
			HaveList: true,
			List:     slice,
		}, nil
	}

	return nil, fmt.Errorf("only primitive types, map & slices can have tag value, not %q", f.v.Kind())
}

func valueToValue(v reflect.Value, opt *marshalOptions) (*Value, error) {
	// Special cased types.
	t := v.Type()
	if t == durationType {
		s := v.Interface().(time.Duration).String()
		return &Value{Str: &s}, nil
	} else if uv, ok := implements(v, textMarshalerInterface); ok {
		tm := uv.Interface().(encoding.TextMarshaler)
		b, err := tm.MarshalText()
		if err != nil {
			return nil, err
		}
		s := string(b)
		return &Value{Str: &s}, nil
	} else if uv, ok := implements(v, jsonMarshalerInterface); ok {
		jm := uv.Interface().(json.Marshaler)
		b, err := jm.MarshalJSON()
		if err != nil {
			return nil, err
		}
		s := string(b)
		return &Value{Str: &s}, nil
	}
	switch t.Kind() {
	case reflect.String:
		s := v.Interface().(string)
		if opt.hereDocsForMultiline == 0 || strings.Count(s, "\n") < opt.hereDocsForMultiline {
			return &Value{Str: &s}, nil
		}
		s = "\n" + s
		return &Value{HeredocDelimiter: "-EOF", Heredoc: &s}, nil

	case reflect.Slice:
		list := []*Value{}
		for i := 0; i < v.Len(); i++ {
			el := v.Index(i)
			elv, err := valueToValue(el, opt)
			if err != nil {
				return nil, err
			}
			list = append(list, elv)
		}
		return &Value{List: list, HaveList: true}, nil

	case reflect.Map:
		entries := []*MapEntry{}
		sorted := []reflect.Value{}
		for _, key := range v.MapKeys() {
			sorted = append(sorted, key)
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].String() < sorted[j].String()
		})
		for _, key := range sorted {
			value, err := valueToValue(v.MapIndex(key), opt)
			if err != nil {
				return nil, err
			}
			keyStr := key.String()
			entries = append(entries, &MapEntry{
				Key:   &Value{Str: &keyStr},
				Value: value,
			})
		}
		return &Value{Map: entries, HaveMap: true}, nil

	case reflect.Float32, reflect.Float64:
		return &Value{Number: big.NewFloat(v.Float())}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Value{Number: big.NewFloat(0).SetInt64(v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Value{Number: big.NewFloat(0).SetUint64(v.Uint())}, nil

	case reflect.Bool:
		b := v.Bool()
		return &Value{Bool: (*Bool)(&b)}, nil

	default:
		switch t {
		case timeType:
			s := v.Interface().(time.Time).Format(time.RFC3339)
			return &Value{Str: &s}, nil

		default:
			panic(t.String())
		}
	}
}

func valueToBlock(v reflect.Value, tag tag, schema bool, opt *marshalOptions) (*Block, error) {
	block := &Block{
		Name:     tag.name,
		Comments: tag.comments(),
	}
	var err error
	block.Body, block.Labels, err = structToEntries(v, schema, opt)
	return block, err
}

func sliceToBlocks(sv reflect.Value, tag tag, opt *marshalOptions) ([]*Block, error) {
	blocks := []*Block{}
	for i := 0; i != sv.Len(); i++ {
		block, err := valueToBlock(sv.Index(i), tag, false, opt)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func marshalNode(w io.Writer, indent string, node Node) error {
	switch node := node.(type) {
	case *AST:
		return marshalAST(w, indent, node)
	case *Block:
		return marshalBlock(w, indent, node)
	case *Attribute:
		return marshalAttribute(w, indent, node)
	case *Value:
		return marshalValue(w, indent, node)
	default:
		return fmt.Errorf("can't marshal node of type %T", node)
	}
}

func marshalAST(w io.Writer, indent string, node *AST) error {
	err := marshalEntries(w, indent, node.Entries)
	if err != nil {
		return err
	}
	marshalComments(w, indent, node.TrailingComments)
	return nil
}

func marshalEntries(w io.Writer, indent string, entries []*Entry) error {
	prevAttr := true
	for i, entry := range entries {
		if block := entry.Block; block != nil {
			if i > 0 {
				fmt.Fprintln(w)
			}
			if err := marshalBlock(w, indent, block); err != nil {
				return err
			}
			prevAttr = false
		} else if attr := entry.Attribute; attr != nil {
			if !prevAttr {
				fmt.Fprintln(w)
			}
			if err := marshalAttribute(w, indent, attr); err != nil {
				return err
			}
			prevAttr = true
		} else {
			panic("??")
		}
	}
	return nil
}

func marshalAttribute(w io.Writer, indent string, attribute *Attribute) error {
	marshalComments(w, indent, attribute.Comments)
	fmt.Fprintf(w, "%s%s = ", indent, attribute.Key)
	err := marshalValue(w, indent, attribute.Value)
	if err != nil {
		return err
	}
	if attribute.Optional {
		fmt.Fprint(w, " // (optional)")
	}
	fmt.Fprintln(w)
	return nil
}

func marshalValue(w io.Writer, indent string, value *Value) error {
	if value.HaveMap {
		return marshalMap(w, indent+"  ", value.Map)
	}
	fmt.Fprintf(w, "%s", value)
	return nil
}

func marshalMap(w io.Writer, indent string, entries []*MapEntry) error {
	fmt.Fprintln(w, "{")
	for _, entry := range entries {
		marshalComments(w, indent, entry.Comments)
		fmt.Fprintf(w, "%s%s: ", indent, entry.Key)
		if err := marshalValue(w, indent+"  ", entry.Value); err != nil {
			return err
		}
		fmt.Fprintln(w, ",")
	}
	fmt.Fprintf(w, "%s}", indent[:len(indent)-2])
	return nil
}

func marshalBlock(w io.Writer, indent string, block *Block) error {
	marshalComments(w, indent, block.Comments)
	fmt.Fprintf(w, "%s%s ", indent, block.Name)
	for _, label := range block.Labels {
		fmt.Fprintf(w, "%q ", label)
	}
	if block.Repeated {
		fmt.Fprintln(w, "{ // (repeated)")
	} else {
		fmt.Fprintln(w, "{")
	}
	err := marshalEntries(w, indent+"  ", block.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func marshalComments(w io.Writer, indent string, comments []string) {
	for _, comment := range comments {
		for _, line := range strings.Split(comment, "\n") {
			fmt.Fprintf(w, "%s// %s\n", indent, line)
		}
	}
}
