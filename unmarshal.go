package hcl

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

var (
	textUnmarshalerInterface = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	textMarshalerInterface   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	jsonUnmarshalerInterface = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	jsonMarshalerInterface   = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	remainType               = reflect.TypeOf([]Entry{})
	durationType             = reflect.TypeOf(time.Duration(0))
	timeType                 = reflect.TypeOf(time.Time{})
)

// Unmarshal HCL into a Go struct.
func Unmarshal(data []byte, v interface{}, options ...MarshalOption) error {
	ast, err := ParseBytes(data)
	if err != nil {
		return err
	}
	return UnmarshalAST(ast, v, options...)
}

// UnmarshalAST unmarshalls an already parsed or constructed AST into a Go struct.
func UnmarshalAST(ast *AST, v interface{}, options ...MarshalOption) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return fmt.Errorf("can't unmarshal into nil")
	}
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("can only unmarshal into a pointer to a struct, not %s", rv.Type())
	}
	opt := &marshalState{}
	for _, option := range options {
		option(opt)
	}
	return unmarshalEntries(rv.Elem(), ast.Entries, opt)
}

// UnmarshalBlock into a struct.
func UnmarshalBlock(block *Block, v interface{}, options ...MarshalOption) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return fmt.Errorf("can't unmarshal into nil")
	}
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("can only unmarshal into a pointer to a struct, not %s", rv)
	}
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%T must be a pointer to a struct", v)
	}
	rv = rv.Elem()
	opt := &marshalState{}
	for _, option := range options {
		option(opt)
	}
	return unmarshalBlock(rv, block, opt)
}

func unmarshalEntries(v reflect.Value, entries []Entry, opt *marshalState) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("%s must be a struct", v.Type())
	}
	// Collect entries from the source into a map.
	seen := map[string]Entry{}
	mentries := make(map[string][]Entry, len(entries))
	for _, entry := range entries {
		key := entry.EntryKey()
		existing, ok := mentries[key]
		if ok {
			_, existingIsBlock := existing[0].(*Block)
			_, newIsBlock := entry.(*Block)
			// Mismatch in type.
			if existingIsBlock != newIsBlock {
				return participle.Errorf(existing[0].Position(), "%s: %s cannot be both block and attribute", entry.Position(), key)
			}
		}
		mentries[key] = append(mentries[key], entry)
		seen[key] = entry
	}
	// Collect the fields of the target struct.
	fields, err := flattenFields(v, opt)
	if err != nil {
		return err
	}
	// Apply HCL entries to our fields.
	for _, field := range fields {
		tag := field.tag // nolint: govet
		switch {
		case tag.name == "":
			continue

		case tag.label:
			delete(seen, tag.name)
			continue

		case tag.remain:
			if field.t.Type != remainType {
				panic(fmt.Sprintf(`"remain" field %q must be of type []*hcl.Entry but is %T`, field.t.Name, field.t.Type))
			}
			var remaining []Entry
			for _, entries := range mentries {
				remaining = append(remaining, entries...)
			}
			sort.Slice(remaining, func(i, j int) bool {
				return remaining[i].EntryKey() < remaining[j].EntryKey()
			})
			field.v.Set(reflect.ValueOf(remaining))
			return nil

		}

		haventSeen := seen[tag.name] == nil
		entries := mentries[tag.name]
		if len(entries) == 0 {
			if !tag.optional && haventSeen {
				return fmt.Errorf("missing required attribute %q", tag.name)
			}
			// apply defaults here as there's no value for this field
			v, err := defaultValueFromTag(field, tag.defaultValue)
			if err != nil {
				return err
			}
			if v != nil {
				// check enum before assigning default value
				err := checkEnum(v, field, tag.enum)
				if err != nil {
					return fmt.Errorf("default value conflicts with enum: %v", err)
				}
				err = unmarshalValue(field.v, v, opt)
				if err != nil {
					return fmt.Errorf("error applying default value to field %q, %v", field.t.Name, err)
				}
			}

			continue
		}
		delete(seen, tag.name)

		entry := entries[0]
		entries = entries[1:]
		mentries[tag.name] = entries

		// Field is a pointer, create value if necessary, then move field down.
		if field.v.Kind() == reflect.Ptr {
			if field.v.IsNil() {
				field.v.Set(reflect.New(field.v.Type().Elem()))
			}
			field.v = field.v.Elem()
			field.t.Type = field.t.Type.Elem()
		}

		// Check for unmarshaler interfaces and other special cases.
		if entry, ok := entry.(*Attribute); ok {
			val, isString := entry.Value.(*String)
			if uv, ok := implements(field.v, jsonUnmarshalerInterface); ok {
				err := uv.Interface().(json.Unmarshaler).UnmarshalJSON([]byte(val.String()))
				if err != nil {
					return participle.Wrapf(val.Position(), err, "invalid value")
				}
				continue
			} else if uv, ok := implements(field.v, textUnmarshalerInterface); ok && isString {
				err := uv.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(val.Str))
				if err != nil {
					return participle.Wrapf(val.Position(), err, "invalid value")
				}
				continue
			} else if val != nil && isString {
				switch field.v.Interface().(type) {
				case time.Duration:
					d, err := time.ParseDuration(val.Str)
					if err != nil {
						return participle.Wrapf(val.Position(), err, "invalid duration")
					}
					field.v.Set(reflect.ValueOf(d))
					continue

				case time.Time:
					t, err := time.Parse(time.RFC3339, val.Str)
					if err != nil {
						return participle.Wrapf(val.Position(), err, "invalid time")
					}
					field.v.Set(reflect.ValueOf(t))
					continue
				}
			}
		}

		switch field.v.Kind() {
		case reflect.Struct:
			if len(entries) > 0 {
				return participle.Errorf(entry.Position(), "duplicate field %q at %s", entry.EntryKey(), entry.Position())
			}
			if entry, ok := entry.(*Attribute); ok {
				return participle.Errorf(entry.Pos, "expected a block for %q but got an attribute", tag.name)
			}
			err := unmarshalBlock(field.v, entry.(*Block), opt)
			if err != nil {
				return participle.Wrapf(entry.Position(), err, "failed to unmarshal block")
			}

		case reflect.Slice:
			// Slice of blocks.
			ptr := false
			elt := field.v.Type().Elem()
			if elt.Kind() == reflect.Ptr {
				elt = elt.Elem()
				ptr = true
			}

			if elt.Kind() == reflect.Struct {
				mentries[field.t.Name] = nil
				entries = append([]Entry{entry}, entries...)
				for _, entry := range entries {
					if entry, ok := entry.(*Attribute); ok {
						return participle.Errorf(entry.Pos, "expected a block for %q but got an attribute", tag.name)
					}
					el := reflect.New(elt).Elem()
					err := unmarshalBlock(el, entry.(*Block), opt)
					if err != nil {
						return participle.Wrapf(entry.Position(), err, "failed to unmarshal block")
					}
					if ptr {
						el = el.Addr()
					}
					field.v.Set(reflect.Append(field.v, el))
				}
				// Remove all entries for a slice of struct after processing
				mentries[tag.name] = nil
				continue
			}
			fallthrough

		default:
			// Anything else must be a scalar value.
			if len(entries) > 0 {
				return participle.Errorf(entry.Position(), "duplicate field %q at %s", entry.EntryKey(), entries[0].Position())
			}
			if _, ok := entry.(*Block); ok {
				return participle.Errorf(entry.Position(), "expected an attribute for %q but got a block", tag.name)
			}
			entry := entry.(*Attribute)
			value := entry.Value
			// check enum before unmarshalling actual value
			err := checkEnum(value, field, tag.enum)
			if err != nil {
				return err
			}
			err = unmarshalValue(field.v, value, opt)
			if err != nil {
				pos := entry.Pos
				if value != nil {
					pos = value.Position()
				}
				return participle.Wrapf(pos, err, "failed to unmarshal value")
			}
		}
	}

	if !opt.allowExtra && len(seen) > 0 {
		need := make([]string, 0, len(seen))
		var pos lexer.Position
		for key, entry := range seen {
			if pos.Column == 0 {
				pos = entry.Position()
			}
			need = append(need, strconv.Quote(key))
		}
		return participle.Errorf(pos, "found extra fields %s", strings.Join(need, ", "))
	}
	return nil
}

func checkEnum(v Value, f field, enum string) error {
	if enum == "" || v == nil {
		return nil
	}

	k := f.v.Kind()
	if k == reflect.Ptr {
		k = f.v.Elem().Kind()
	}

	switch k {
	case reflect.Map, reflect.Struct, reflect.Array, reflect.Slice:
		return fmt.Errorf("enum on map, struct, array and slice are not supported on field %q", f.t.Name)
	default:
		enums, err := enumValuesFromTag(f, enum)
		if err != nil {
			return err
		}
		enumStr := []string{}
		for _, e := range enums {
			if e.String() == v.String() {
				return nil
			}
			enumStr = append(enumStr, e.String())
		}

		return fmt.Errorf("value %s does not match anything within enum %s", v.String(), strings.Join(enumStr, ", "))
	}
}

func unmarshalBlock(v reflect.Value, block *Block, opt *marshalState) error {
	if pos := v.FieldByName("Pos"); pos.IsValid() {
		pos.Set(reflect.ValueOf(block.Pos))
	}
	fields, err := flattenFields(v, opt)
	if err != nil {
		return participle.Wrapf(block.Pos, err, "")
	}
	labels := block.Labels
	for _, field := range fields {
		tag := field.tag // nolint: govet
		if tag.name == "" || !tag.label {
			continue
		}
		if len(labels) == 0 {
			return participle.Errorf(block.Pos, "missing label %q", tag.name)
		}
		if uv, ok := implements(field.v, textUnmarshalerInterface); ok {
			label := labels[0]
			labels = labels[1:]
			err := uv.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(label))
			if err != nil {
				return participle.Wrapf(block.Pos, err, "invalid label %q", tag.name)
			}
		} else if field.v.Kind() == reflect.String {
			label := labels[0]
			labels = labels[1:]
			field.v.SetString(label)
		} else if field.v.Kind() == reflect.Slice && field.v.Type().Elem().Kind() == reflect.String {
			field.v.Set(reflect.ValueOf(labels))
			labels = nil
		} else {
			panic("label field " + fieldID(v.Type(), field.t) + " must be string or []string")
		}
	}
	if len(labels) > 0 {
		return participle.Errorf(block.Pos, "too many labels for block %q", block.Name)
	}
	return unmarshalEntries(v, block.Body, opt)
}

func unmarshalValue(rv reflect.Value, v Value, opt *marshalState) error {
	switch rv.Kind() {
	case reflect.String:
		switch v := v.(type) {
		case *String:
			rv.SetString(v.Str)
		case *Type:
			rv.SetString(v.Type)
		case *Heredoc:
			rv.SetString(v.GetHeredoc())
		default:
			return participle.Errorf(v.Position(), "expected a type or string but got %s", v)
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Int64()
		rv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Uint64()
		rv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Float64()
		rv.SetFloat(n)

	case reflect.Map:
		mapping, ok := v.(*Map)
		if !ok {
			return participle.Errorf(v.Position(), "expected a map but got %s", v)
		}
		t := rv.Type()
		rv.Set(reflect.MakeMap(t))
		for _, entry := range mapping.Entries {
			key := reflect.New(t.Key()).Elem()
			err := unmarshalMapKey(key, entry.Key)
			if err != nil {
				return participle.Wrapf(entry.Key.Position(), err, "invalid map key")
			}
			value := reflect.New(t.Elem()).Elem()
			err = unmarshalValue(value, entry.Value, opt)
			if err != nil {
				return participle.Wrapf(entry.Value.Position(), err, "invalid map value")
			}
			rv.SetMapIndex(key, value)
		}

	case reflect.Slice:
		list, ok := v.(*List)
		if !ok {
			return participle.Errorf(v.Position(), "expected a list but got %s", v)
		}
		t := rv.Type().Elem()
		lv := reflect.MakeSlice(rv.Type(), 0, 4)
		for _, entry := range list.List {
			value := reflect.New(t).Elem()
			err := unmarshalValue(value, entry, opt)
			if err != nil {
				return participle.Wrapf(entry.Position(), err, "invalid list element")
			}
			lv = reflect.Append(lv, value)
		}
		rv.Set(lv)

	case reflect.Ptr:
		if rv.IsNil() {
			pv := reflect.New(rv.Type().Elem())
			rv.Set(pv)
		}
		return unmarshalValue(rv.Elem(), v, opt)

	case reflect.Bool:
		var value bool
		if v == nil {
			if !opt.bareAttr {
				return fmt.Errorf("expected = after attribute")
			}
			value = true
		} else if b, ok := v.(*Bool); ok {
			value = b.Bool
		} else {
			return participle.Errorf(v.Position(), "expected a bool but got %s", v)
		}
		rv.SetBool(value)

	case reflect.Interface:
		err := unmarshalAny(rv, v, opt)
		if err != nil {
			return participle.Wrapf(v.Position(), err, "invalid interface target")
		}

	default:
		panic(rv.Kind().String())
	}
	return nil
}

func unmarshalMapKey(rv reflect.Value, v Value) error {
	switch rv.Kind() {
	case reflect.String:
		switch v := v.(type) {
		case *String:
			rv.SetString(v.Str)
		case *Type:
			rv.SetString(v.Type)
		case *Heredoc:
			rv.SetString(v.GetHeredoc())
		default:
			return participle.Errorf(v.Position(), "expected a type or string but got %s", v)
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Int64()
		rv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Uint64()
		rv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		number, ok := v.(*Number)
		if !ok {
			return participle.Errorf(v.Position(), "expected a number but got %s", v)
		}
		n, _ := number.Float.Float64()
		rv.SetFloat(n)

	default:
		panic(fmt.Sprintf("unsupported type for map keys: %s", rv.Kind().String()))
	}
	return nil
}

func unmarshalAny(rv reflect.Value, v Value, state *marshalState) error {
	if rv.Kind() != reflect.Interface {
		return fmt.Errorf("can only unmarshall any to an interface{}/any receiver")
	}
	if rv.NumMethod() != 0 {
		return participle.Errorf(v.Position(), "expected any/interface{} but got %s", rv.Type().String())
	}

	var empty any
	switch y := v.(type) {
	case *Number:
		f, _ := y.Float.Float64()
		rv.Set(reflect.ValueOf(f))
	case *Bool:
		rv.Set(reflect.ValueOf(y.Bool))
	case *String:
		rv.Set(reflect.ValueOf(y.Str))
	case *Map:
		receivingMap := reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(&empty).Elem())
		mapPtr := reflect.New(receivingMap).Elem()
		err := unmarshalValue(mapPtr, v, state)
		if err != nil {
			return participle.Wrapf(v.Position(), err, "bad map")
		}
		rv.Set(mapPtr)
	case *List:
		receivingSlice := reflect.SliceOf(reflect.TypeOf(&empty).Elem())
		slicePtr := reflect.New(receivingSlice).Elem()
		err := unmarshalValue(slicePtr, v, state)
		if err != nil {
			return participle.Wrapf(v.Position(), err, "bad slice")
		}
		rv.Set(slicePtr)
	default:
		return participle.Errorf(v.Position(), "can't unmarshall %s to any/interface{}", v.String())
	}
	return nil
}

type field struct {
	t   reflect.StructField
	v   reflect.Value
	tag tag
}

func flattenFields(v reflect.Value, opt *marshalState) ([]field, error) {
	out := make([]field, 0, v.NumField())
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		ft := t.Field(i)
		if ft.Anonymous {
			if f.Kind() != reflect.Struct {
				return nil, fmt.Errorf("%s: anonymous field must be a struct", ft.Name)
			}
			sub, err := flattenFields(f, opt)
			if err != nil {
				return nil, fmt.Errorf("%s: %s", ft.Name, err)
			}
			out = append(out, sub...)
		} else {
			tag := parseTag(v.Type(), ft, opt) // nolint: govet
			out = append(out, field{ft, f, tag})
		}
	}
	return out, nil
}

func fieldID(parent reflect.Type, t reflect.StructField) string {
	return fmt.Sprintf("%s.%s.%s", parent.PkgPath(), parent.Name(), t.Name)
}

type tag struct {
	name         string
	optional     bool
	label        bool
	block        bool
	remain       bool
	help         string
	defaultValue string
	enum         string
}

func (t tag) comments(opts *marshalState) []string {
	if opts.schemaComments || opts.schema {
		var lines []string
		if t.help != "" {
			lines = append(lines, strings.Split(t.help, "\n")...)
		}
		return lines
	}
	return nil
}

func parseTag(parent reflect.Type, t reflect.StructField, opt *marshalState) tag {
	help := t.Tag.Get("help")
	defaultValue := t.Tag.Get("default")
	enum := t.Tag.Get("enum")
	s, ok := t.Tag.Lookup("hcl")

	isBlock := false
	if !ok && opt.inferHCLTags {
		// if the struct field is a struct or pointer to struct set the tag as block
		tt := t.Type
		for tt.Kind() == reflect.Ptr || tt.Kind() == reflect.Slice {
			tt = tt.Elem()
		}
		isBlock = tt.Kind() == reflect.Struct
	}

	if !ok {
		s, ok = t.Tag.Lookup("json")
		if !ok {
			return tag{name: t.Name, block: isBlock, optional: true, help: help, defaultValue: defaultValue, enum: enum}
		}
	}
	parts := strings.Split(s, ",")
	name := parts[0]
	if name == "-" {
		return tag{}
	}
	id := fieldID(parent, t)
	if name == "" {
		name = t.Name
	}
	if len(parts) == 1 {
		return tag{name: name, block: isBlock, help: help, defaultValue: defaultValue, optional: defaultValue != "", enum: enum}
	}
	option := parts[1]
	switch option {
	case "optional", "omitempty":
		return tag{name: name, block: isBlock, optional: true, help: help, defaultValue: defaultValue, enum: enum}
	case "label":
		return tag{name: name, label: true, help: help}
	case "block":
		return tag{name: name, block: true, optional: true, help: help}
	case "remain":
		return tag{name: name, remain: true, help: help}
	default:
		panic("invalid HCL tag option " + option + " on " + id)
	}
}

func implements(v reflect.Value, iface reflect.Type) (reflect.Value, bool) {
	if v.Type().Implements(iface) {
		return v, true
	} else if v.CanAddr() && v.Addr().Type().Implements(iface) {
		return v.Addr(), true
	}
	return reflect.Value{}, false
}

func typeImplements(t reflect.Type, iface reflect.Type) bool {
	if t.Implements(iface) {
		return true
	} else if reflect.PtrTo(t).Implements(iface) {
		return true
	}
	return false
}
