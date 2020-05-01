package hcl

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var (
	textUnmarshalerInterface = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	textMarshalerInterface   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	jsonUnmarshalerInterface = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	jsonMarshalerInterface   = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	remainType               = reflect.TypeOf([]*Entry{})
	durationType             = reflect.TypeOf(time.Duration(0))
	timeType                 = reflect.TypeOf(time.Time{})
)

// Unmarshal HCL into a Go struct.
func Unmarshal(data []byte, v interface{}) error {
	ast, err := ParseBytes(data)
	if err != nil {
		return err
	}
	return UnmarshalAST(ast, v)
}

// UnmarshalAST unmarshals an already parsed or constructed AST into a Go struct.
func UnmarshalAST(ast *AST, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("%T must be a pointer", v)
	}
	return unmarshalEntries(rv.Elem(), ast.Entries)
}

func unmarshalEntries(v reflect.Value, entries []*Entry) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("%T must be a struct", v.Interface())
	}
	// Collect entries from the source into a map.
	seen := map[string]bool{}
	mentries := make(map[string][]*Entry, len(entries))
	for _, entry := range entries {
		key := entry.Key()
		existing, ok := mentries[key]
		if ok {
			// Mismatch in type.
			if !((existing[0].Block == nil) == (entry.Block == nil)) {
				return fmt.Errorf("%s,%s: %s cannot be both block and attribute", existing[0].Pos, entry.Pos, key)
			}
		}
		mentries[key] = append(mentries[key], entry)
		seen[key] = true
	}
	// Collect the fields of the target struct.
	fields, err := flattenFields(v)
	if err != nil {
		return err
	}
	// Apply HCL entries to our fields.
	for _, field := range fields {
		tag := parseTag(v.Type(), field.t) // nolint: govet
		if tag.name == "" {
			continue
		}
		if tag.label {
			delete(seen, tag.name)
			continue
		}
		if tag.remain {
			if field.t.Type != remainType {
				panic(fmt.Sprintf("\"remain\" field %q must be of type []*hcl.Entry but is %T", field.t.Name, field.t.Type))
			}
			remaining := []*Entry{}
			for _, entries := range mentries {
				remaining = append(remaining, entries...)
			}
			field.v.Set(reflect.ValueOf(remaining))
			return nil
		}
		haventSeen := !seen[tag.name]
		entries := mentries[tag.name]
		if len(entries) == 0 {
			if !tag.optional && haventSeen {
				return fmt.Errorf("missing required attribute %q", tag.name)
			}
			continue
		}
		delete(seen, tag.name)

		entry := entries[0]
		entries = entries[1:]
		mentries[tag.name] = entries

		// Check for unmarshaler interfaces and other special cases.
		if entry.Attribute != nil {
			val := entry.Attribute.Value
			if uv, ok := implements(field.v, jsonUnmarshalerInterface); ok {
				err := uv.Interface().(json.Unmarshaler).UnmarshalJSON([]byte(val.String()))
				if err != nil {
					return fmt.Errorf("%s: invalid value: %s", val.Pos, err)
				}
				continue
			} else if uv, ok := implements(field.v, textUnmarshalerInterface); ok {
				err := uv.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(*val.Str))
				if err != nil {
					return fmt.Errorf("%s: invalid value: %s", val.Pos, err)
				}
				continue
			} else if entry.Attribute.Value.Str != nil {
				switch field.v.Interface().(type) {
				case time.Duration:
					d, err := time.ParseDuration(*val.Str)
					if err != nil {
						return fmt.Errorf("%s: invalid duration: %s", val.Pos, err)
					}
					field.v.Set(reflect.ValueOf(d))
					continue

				case time.Time:
					t, err := time.Parse(time.RFC3339, *val.Str)
					if err != nil {
						return fmt.Errorf("%s: invalid time: %s", val.Pos, err)
					}
					field.v.Set(reflect.ValueOf(t))
					continue
				}
			}
		}

		// Field is a pointer, create value if necessary, then move field down.
		if field.v.Kind() == reflect.Ptr {
			if field.v.IsNil() {
				field.v.Set(reflect.New(field.v.Type().Elem()))
			}
			field.v = field.v.Elem()
			field.t.Type = field.t.Type.Elem()
		}

		switch field.v.Kind() {
		case reflect.Struct:
			if len(entries) > 0 {
				return fmt.Errorf("%s: duplicate field %q at %s", entry.Pos, entry.Key(), entry.Pos)
			}
			if entry.Attribute != nil {
				return fmt.Errorf("%s: expected a block for %q but got an attribute", entry.Pos, tag.name)
			}
			err := unmarshalBlock(field.v, entry.Block)
			if err != nil {
				return fmt.Errorf("%s: %s", entry.Pos, err)
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
				entries = append([]*Entry{entry}, entries...)
				for _, entry := range entries {
					if entry.Attribute != nil {
						return fmt.Errorf("%s: expected a block for %q but got an attribute", entry.Pos, tag.name)
					}
					el := reflect.New(elt).Elem()
					err := unmarshalBlock(el, entry.Block)
					if err != nil {
						return fmt.Errorf("%s: %s", entry.Pos, err)
					}
					if ptr {
						el = el.Addr()
					}
					field.v.Set(reflect.Append(field.v, el))
				}
				continue
			}
			fallthrough

		default:
			// Anything else must be a scalar value.
			if len(entries) > 0 {
				return fmt.Errorf("%s: duplicate field %q at %s", entry.Pos, entry.Key(), entries[0].Pos)
			}
			if entry.Block != nil {
				return fmt.Errorf("%s: expected an attribute for %q but got a block", entry.Pos, tag.name)
			}
			value := entry.Attribute.Value
			err = unmarshalValue(field.v, value)
			if err != nil {
				return fmt.Errorf("%s: %s", value.Pos, err)
			}
		}
	}

	if len(seen) > 0 {
		need := []string{}
		for key := range seen {
			need = append(need, key)
		}
		return fmt.Errorf("found extra fields %s", strings.Join(need, ", "))
	}
	return nil
}

func unmarshalBlock(v reflect.Value, block *Block) error {
	fields, err := flattenFields(v)
	if err != nil {
		return err
	}
	labels := block.Labels
	for _, field := range fields {
		tag := parseTag(v.Type(), field.t) // nolint: govet
		if tag.name == "" || !tag.label {
			continue
		}
		if len(labels) == 0 {
			return fmt.Errorf("missing label %q", tag.name)
		}
		if field.v.Kind() != reflect.String {
			panic("label field " + fieldID(v.Type(), field.t) + " must be a string")
		}
		label := labels[0]
		labels = labels[1:]
		field.v.SetString(label)
	}
	if len(labels) > 0 {
		return fmt.Errorf("too many labels for block %q", block.Name)
	}
	return unmarshalEntries(v, block.Body)
}

func unmarshalValue(rv reflect.Value, v *Value) error {
	switch rv.Kind() {
	case reflect.String:
		if v.Str == nil {
			return fmt.Errorf("expected a string but got %s", v)
		}
		rv.SetString(*v.Str)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Number == nil {
			return fmt.Errorf("expected a number but got %s", v)
		}
		n, _ := v.Number.Int64()
		rv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Number == nil {
			return fmt.Errorf("expected a number but got %s", v)
		}
		n, _ := v.Number.Uint64()
		rv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		if v.Number == nil {
			return fmt.Errorf("expected a number but got %s", v)
		}
		n, _ := v.Number.Float64()
		rv.SetFloat(n)

	case reflect.Map:
		if v.Map == nil {
			return fmt.Errorf("expected a map but got %s", v)
		}
		t := rv.Type()
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("map keys must be strings but we have %s", t.Key()))
		}
		rv.Set(reflect.MakeMap(t))
		for _, entry := range v.Map {
			key := reflect.New(t.Key()).Elem()
			value := reflect.New(t.Elem()).Elem()
			key.SetString(entry.Key)
			err := unmarshalValue(value, entry.Value)
			if err != nil {
				return fmt.Errorf("%s: invalid map value: %s", entry.Value.Pos, err)
			}
			rv.SetMapIndex(key, value)
		}

	case reflect.Slice:
		if v.List == nil {
			return fmt.Errorf("expected a list but got %s", v)
		}
		t := rv.Type().Elem()
		lv := rv
		for _, entry := range v.List {
			value := reflect.New(t).Elem()
			err := unmarshalValue(value, entry)
			if err != nil {
				return fmt.Errorf("%s: invalid list element: %s", entry.Pos, err)
			}
			lv = reflect.Append(lv, value)
		}
		rv.Set(lv)

	case reflect.Ptr:
		if rv.IsNil() {
			pv := reflect.New(rv.Type().Elem())
			rv.Set(pv)
		}
		return unmarshalValue(rv.Elem(), v)

	default:
		panic(rv.Kind().String())
	}
	return nil
}

type field struct {
	t reflect.StructField
	v reflect.Value
}

func flattenFields(v reflect.Value) ([]field, error) {
	out := []field{}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		ft := t.Field(i)
		if ft.Anonymous {
			if ft.Type.Kind() != reflect.Struct {
				return nil, fmt.Errorf("%s: anonymous field must be a struct", ft.Name)
			}
			sub, err := flattenFields(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %s", ft.Name, err)
			}
			out = append(out, sub...)
		} else {
			out = append(out, field{ft, f})
		}
	}
	return out, nil
}

func fieldID(parent reflect.Type, t reflect.StructField) string {
	return fmt.Sprintf("%s.%s.%s", parent.PkgPath(), parent.Name(), t.Name)
}

type tag struct {
	name     string
	optional bool
	label    bool
	block    bool
	remain   bool
}

func parseTag(parent reflect.Type, t reflect.StructField) tag {
	s, ok := t.Tag.Lookup("hcl")
	if !ok {
		s, ok = t.Tag.Lookup("json")
		if !ok {
			return tag{name: t.Name}
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
		return tag{name: name}
	}
	option := parts[1]
	switch option {
	case "optional", "omitempty":
		return tag{name: name, optional: true}
	case "label":
		return tag{name: name, label: true}
	case "block":
		return tag{name: name, block: true, optional: true}
	case "remain":
		return tag{name: name, remain: true}
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
