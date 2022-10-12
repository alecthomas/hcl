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

	"github.com/alecthomas/participle/v2/lexer"
)

// marshalState defines options and state for the marshalling/unmarshalling process
type marshalState struct {
	inferHCLTags         bool
	hereDocsForMultiline int
	bareAttr             bool
	schema               bool
	schemaComments       bool
	seenStructs          map[reflect.Type]bool
	allowExtra           bool
}

// Create a shallow clone with schema overridden.
func (m *marshalState) withSchema(schema bool) *marshalState {
	out := *m
	out.schema = schema
	return &out
}

// MarshalOption configures optional marshalling behaviour.
type MarshalOption func(options *marshalState)

// InferHCLTags specifies whether to infer behaviour if hcl:"" tags are not present.
//
// This currently just means that all structs become blocks.
func InferHCLTags(v bool) MarshalOption {
	return func(options *marshalState) {
		options.inferHCLTags = v
	}
}

// BareBooleanAttributes specifies whether attributes without values will be
// treated as boolean true values.
//
// eg.
//
//	attr
//
// NOTE: This is non-standard HCL.
func BareBooleanAttributes(v bool) MarshalOption {
	return func(options *marshalState) {
		options.bareAttr = true
	}
}

// HereDocsForMultiLine will marshal multi-line strings >= n lines as indented
// heredocs rather than quoted strings.
func HereDocsForMultiLine(n int) MarshalOption {
	return func(options *marshalState) {
		options.hereDocsForMultiline = n
	}
}

// AllowExtra fields in configuration to be skipped.
func AllowExtra(ok bool) MarshalOption {
	return func(options *marshalState) {
		options.allowExtra = true
	}
}

// WithSchemaComments will export the contents of the help struct tag
// as comments when marshaling.
func WithSchemaComments(v bool) MarshalOption {
	return func(options *marshalState) {
		options.schemaComments = v
	}
}

func asSchema(schema bool) MarshalOption {
	return func(options *marshalState) {
		options.schema = schema
	}
}

// newMarshalState creates marshal options from a set of options
func newMarshalState(options ...MarshalOption) *marshalState {
	opt := &marshalState{
		seenStructs: map[reflect.Type]bool{},
	}
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
	return marshalToAST(v, newMarshalState(options...))
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

func marshalToAST(v interface{}, opt *marshalState) (*AST, error) {
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
			Schema: opt.schema,
		}
	)
	ast.Entries, labels, err = structToEntries(rv, opt)
	if err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		return nil, fmt.Errorf("unexpected labels %s at top level", strings.Join(labels, ", "))
	}
	return ast, nil
}

func structToEntries(v reflect.Value, opt *marshalState) (entries []Entry, labels []string, err error) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !opt.schema {
				return nil, nil, nil
			}
			v = reflect.New(v.Type().Elem())
		}
		v = v.Elem()
	}

	// Check for recursive structures.
	if opt.schema && opt.seenStructs[v.Type()] {
		return []Entry{
			&RecursiveEntry{},
		}, nil, nil
	}
	opt.seenStructs[v.Type()] = true
	defer delete(opt.seenStructs, v.Type())

	fields, err := flattenFields(v, opt)
	if err != nil {
		return nil, nil, err
	}
	for _, field := range fields {
		tag := field.tag
		switch {
		case tag.name == "": // Skip
		case tag.label:
			if opt.schema {
				labels = append(labels, tag.name)
			} else {
				if field.v.Kind() == reflect.String {
					labels = append(labels, field.v.String())
				} else if field.v.Kind() == reflect.Slice && field.v.Type().Elem().Kind() == reflect.String {
					labels = append(labels, field.v.Interface().([]string)...)
				}
			}

		case tag.block:
			if field.v.Kind() == reflect.Slice {
				var blocks []*Block
				if opt.schema {
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
					entries = append(entries, block)
				}
			} else if opt.schema || field.v.Kind() != reflect.Ptr || !field.v.IsNil() {
				block, err := valueToBlock(field.v, tag, opt)
				if err != nil {
					return nil, nil, err
				}
				entries = append(entries, block)
			}

		default:
			attr, err := fieldToAttr(field, tag, opt)
			if err != nil {
				return nil, nil, err
			}
			hasDefaultAndEqualsValue := attr.Default != nil && attr.Value != nil && attr.Value.String() == attr.Default.String()
			noDefaultButIsZero := attr.Default == nil && field.v.IsZero()
			valueEqualsDefault := noDefaultButIsZero || hasDefaultAndEqualsValue
			if tag.optional {
				if attr.Value == nil || (!opt.schema && valueEqualsDefault) {
					continue
				}
			} else if attr.Value == nil {
				return nil, nil, fmt.Errorf("required value cannot be nil")
			}
			entries = append(entries, attr)
		}
	}
	return entries, labels, nil
}

func fieldToAttr(field field, tag tag, opt *marshalState) (*Attribute, error) {
	attr := &Attribute{
		Key:      tag.name,
		Comments: tag.comments(opt),
	}
	if opt.schemaComments {
		if tag.enum != "" {
			attr.Comments = append(attr.Comments, fmt.Sprintf("enum: %s", tag.enum))
		}
		if tag.defaultValue != "" {
			attr.Comments = append(attr.Comments, fmt.Sprintf("default: %s", tag.defaultValue))
		}
	}
	var err error
	if opt.schema {
		attr.Value, err = attrSchema(field.v.Type())
	} else if !(field.v.Kind() == reflect.Ptr && field.v.IsNil()) {
		attr.Value, err = valueToValue(field.v, opt)
	}
	if err != nil {
		return nil, err
	}
	attr.Default, err = defaultValueFromTag(field, tag.defaultValue)
	if err != nil {
		return nil, err
	}
	attr.Optional = (tag.optional || attr.Default != nil) && opt.schema
	attr.Enum, err = enumValuesFromTag(field, tag.enum)
	return attr, err
}

func defaultValueFromTag(f field, defaultValue string) (Value, error) {
	v, err := valueFromTag(f, defaultValue)
	if err != nil {
		return nil, fmt.Errorf("error parsing default value: %v", err)
	}
	return v, nil
}

// enumValuesFromTag parses the enum string from tag into a list of Values
func enumValuesFromTag(f field, enum string) ([]Value, error) {
	if enum == "" {
		return nil, nil
	}

	enums := strings.Split(enum, ",")
	list := make([]Value, 0, len(enums))
	for _, e := range enums {
		enumVal, err := valueFromTag(f, e)
		if err != nil {
			return nil, fmt.Errorf("error parsing enum: %v", err)
		}

		list = append(list, enumVal)
	}

	return list, nil

}

func valueFromTag(f field, defaultValue string) (Value, error) {
	if defaultValue == "" {
		return nil, nil // nolint: nilnil
	}

	k := f.v.Kind()
	if k == reflect.Ptr {
		k = f.v.Type().Elem().Kind()
	}

	t := f.v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if typeImplements(t, textMarshalerInterface) || t == durationType || t == timeType {
		return &String{Str: defaultValue}, nil
	}

	switch k {
	case reflect.String:
		return &String{Str: defaultValue}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(defaultValue, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to int", defaultValue)
		}
		return &Number{Float: big.NewFloat(0).SetInt64(n)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(defaultValue, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to uint", defaultValue)
		}
		return &Number{Float: big.NewFloat(0).SetUint64(n)}, nil
	case reflect.Float32, reflect.Float64:
		size := 64
		if k == reflect.Float32 {
			size = 32
		}
		n, err := strconv.ParseFloat(defaultValue, size)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to float", defaultValue)
		}
		return &Number{Float: big.NewFloat(n)}, nil
	case reflect.Bool:
		b, err := strconv.ParseBool(defaultValue)
		if err != nil {
			return nil, fmt.Errorf("error converting %q to bool", defaultValue)
		}
		return &Bool{Bool: b}, nil
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
			key := &String{Str: pair[0]}
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

		return &Map{Entries: mapEntries}, nil
	case reflect.Slice:
		slice := []Value{}
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

		return &List{List: slice}, nil
	default:
		return nil, fmt.Errorf("only primitive types, map & slices can have tag value, not %q", f.v.Type())
	}
}

func valueToValue(v reflect.Value, opt *marshalState) (Value, error) {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	// Special cased types.
	t := v.Type()
	if t == durationType {
		s := v.Interface().(time.Duration).String()
		return &String{Str: s}, nil
	} else if uv, ok := implements(v, textMarshalerInterface); ok {
		tm := uv.Interface().(encoding.TextMarshaler)
		b, err := tm.MarshalText()
		if err != nil {
			return nil, err
		}
		s := string(b)
		return &String{Str: s}, nil
	} else if uv, ok := implements(v, jsonMarshalerInterface); ok {
		jm := uv.Interface().(json.Marshaler)
		b, err := jm.MarshalJSON()
		if err != nil {
			return nil, err
		}
		s := string(b)
		return &String{Str: s}, nil
	}
	switch t.Kind() {
	case reflect.String:
		s := v.Interface().(string)
		if opt.hereDocsForMultiline == 0 || strings.Count(s, "\n") < opt.hereDocsForMultiline {
			return &String{Str: s}, nil
		}
		s = "\n" + s
		return &Heredoc{Delimiter: "-EOF", Doc: s}, nil

	case reflect.Slice:
		list := []Value{}
		for i := 0; i < v.Len(); i++ {
			el := v.Index(i)
			elv, err := valueToValue(el, opt)
			if err != nil {
				return nil, err
			}
			list = append(list, elv)
		}
		return &List{List: list}, nil

	case reflect.Map:
		entries := []*MapEntry{}
		sorted := v.MapKeys()
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
				Key:   &String{Str: keyStr},
				Value: value,
			})
		}
		return &Map{Entries: entries}, nil

	case reflect.Float32, reflect.Float64:
		return &Number{Float: big.NewFloat(v.Float())}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Number{Float: big.NewFloat(0).SetInt64(v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Number{Float: big.NewFloat(0).SetUint64(v.Uint())}, nil

	case reflect.Bool:
		return &Bool{Bool: v.Bool()}, nil

	default:
		switch t {
		case timeType:
			s := v.Interface().(time.Time).Format(time.RFC3339)
			return &String{Str: s}, nil

		default:
			panic(t.String())
		}
	}
}

func valueToBlock(v reflect.Value, tag tag, opt *marshalState) (*Block, error) {
	block := &Block{
		Name:     tag.name,
		Comments: tag.comments(opt),
	}
	var err error
	block.Body, block.Labels, err = structToEntries(v, opt)
	return block, err
}

func sliceToBlocks(sv reflect.Value, tag tag, opt *marshalState) ([]*Block, error) {
	blocks := []*Block{}
	for i := 0; i != sv.Len(); i++ {
		block, err := valueToBlock(sv.Index(i), tag, opt.withSchema(false))
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
	case Value:
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

func marshalEntries(w io.Writer, indent string, entries []Entry) error {
	prevAttr := true
	for i, entry := range entries {
		switch entry := entry.(type) {
		case *Block:
			if i > 0 {
				fmt.Fprintln(w)
			}
			if err := marshalBlock(w, indent, entry); err != nil {
				return err
			}
			prevAttr = false

		case *Attribute:
			if !prevAttr {
				fmt.Fprintln(w)
			}
			if err := marshalAttribute(w, indent, entry); err != nil {
				return err
			}
			prevAttr = true

		case *RecursiveEntry:
			fmt.Fprintf(w, "%s// (recursive)\n", indent)

		default:
			panic("??")
		}
	}
	return nil
}

func marshalAttribute(w io.Writer, indent string, attribute *Attribute) error {
	marshalComments(w, indent, attribute.Comments)
	fmt.Fprintf(w, "%s%s = ", indent, attribute.Key)
	vw := &strings.Builder{}
	err := marshalValue(vw, indent, attribute.Value)
	if err != nil {
		return err
	}
	constraints := []string{}
	if isType(attribute.Value) {
		if attribute.Optional {
			constraints = append(constraints, "optional")
		}
		if attribute.Default != nil {
			constraints = append(constraints, fmt.Sprintf("default(%s)", attribute.Default))
		}
		if len(attribute.Enum) > 0 {
			enum := []string{}
			for _, v := range attribute.Enum {
				enum = append(enum, v.String())
			}
			constraints = append(constraints, fmt.Sprintf("enum(%s)", strings.Join(enum, ", ")))
		}
	}
	fmt.Fprint(w, vw)
	if len(constraints) > 0 {
		fmt.Fprintf(w, "(%s)", strings.Join(constraints, " "))
	}
	fmt.Fprintln(w)
	return nil
}

func isType(value Value) bool {
	switch value := value.(type) {
	case *Type:
		return true

	case *List:
		return len(value.List) == 1 && isType(value.List[0])

	case *Map:
		return len(value.Entries) == 1 && isType(value.Entries[0].Value)

	default:
		return false
	}
}

func marshalValue(w io.Writer, indent string, value Value) error {
	if value, ok := value.(*Map); ok {
		return marshalMap(w, indent+"  ", value.Entries)
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
	prefix := fmt.Sprintf("%s%s", indent, block.Name)
	fmt.Fprint(w, prefix)
	if block.Repeated {
		fmt.Fprint(w, "(repeated)")
	}
	labelIndent := len(prefix)
	size := labelIndent
	for i, label := range block.Labels {
		text := strconv.Quote(label)
		size += len(text)
		if i > 0 && size+2 >= 80 {
			size = labelIndent
			fmt.Fprintf(w, "\n %s", strings.Repeat(" ", labelIndent))
		} else {
			fmt.Fprintf(w, " ")
		}
		fmt.Fprintf(w, "%s", text)
	}
	fmt.Fprintln(w, " {")
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
