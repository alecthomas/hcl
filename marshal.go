package hcl

import (
	"bytes"
	"fmt"
	"io"
)

func Marshal(v interface{}) ([]byte, error) {
	ast, err := MarshalToAST(v)
	if err != nil {
		return nil, err
	}
	return MarshalAST(ast)
}

func MarshalToAST(v interface{}) (*AST, error) {
	return &AST{}, nil
}

// MarshalAST marshals an AST to HCL.
func MarshalAST(ast *AST) ([]byte, error) {
	w := &bytes.Buffer{}
	err := MarshalASTToWriter(ast, w)
	return w.Bytes(), err
}

func MarshalASTToWriter(ast *AST, w io.Writer) error {
	return marshalEntries(w, "", ast.Entries)
}

func marshalEntries(w io.Writer, indent string, entries []*Entry) error {
	for i, entry := range entries {
		marshalComments(w, indent, entry.Comments)
		if entry.Block != nil {
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
