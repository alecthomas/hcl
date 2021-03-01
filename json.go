package hcl

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/repr"
)

// MarshalJSONOption implementations control how JSON is marshalled.
type MarshalJSONOption func(o *marshalJSONOptions)

// IncludeComments includes comments as __comments__ attributes in the generated JSON.
func IncludeComments(ok bool) MarshalJSONOption {
	return func(o *marshalJSONOptions) { o.comments = ok }
}

// marshalJSONOptions controls how custom JSON marshaling is applied.
type marshalJSONOptions struct {
	// Include comments as JSON attributes.
	comments bool
}

// MarshalJSON gives fine-grained control over JSON marshaling of an AST.
//
// Currently this just means that emission of comments can be controlled.
func MarshalJSON(ast *AST, options ...MarshalJSONOption) ([]byte, error) {
	m := &jsonVisitor{
		Buffer: &bytes.Buffer{},
	}
	for _, option := range options {
		option(&m.marshalJSONOptions)
	}
	err := Visit(ast, m.Visit)
	return m.Bytes(), err
}

func (a *AST) MarshalJSON() ([]byte, error) {
	if a.Schema {
		return json.Marshal((*rawAST)(a))
	}
	return MarshalJSON(a)
}

type rawAST AST

type jsonVisitor struct {
	*bytes.Buffer
	marshalJSONOptions
}

func (w *jsonVisitor) Visit(node Node, next func() error) error {
	switch node := node.(type) {
	case *AST:
		fmt.Fprint(w, "{")
		for i, entry := range node.Entries {
			if i != 0 {
				fmt.Fprint(w, ",")
			}
			if err := Visit(entry, w.Visit); err != nil {
				return err
			}
		}
		fmt.Fprint(w, "}")
		return nil

	case *Block:
		fmt.Fprintf(w, "%q:{", node.Name)
		if w.comments && len(node.Comments) > 0 {
			fmt.Fprint(w, `"__comments__":`)
			if err := json.NewEncoder(w).Encode(node.Comments); err != nil {
				return err
			}
			fmt.Fprint(w, `,`)
		}
		for _, label := range node.Labels {
			fmt.Fprintf(w, "%q:{", label)
		}
		for i, entry := range node.Body {
			if i != 0 {
				fmt.Fprint(w, ",")
			}
			if err := Visit(entry, w.Visit); err != nil {
				return err
			}
		}
		for range node.Labels {
			fmt.Fprint(w, "}")
		}
		fmt.Fprint(w, "}")
		return nil

	case *Attribute:
		if w.comments && len(node.Comments) > 0 {
			fmt.Fprintf(w, `"__%s_comments__":`, node.Key)
			if err := json.NewEncoder(w).Encode(node.Comments); err != nil {
				return err
			}
			fmt.Fprint(w, `,`)
		}
		fmt.Fprintf(w, "%q:", node.Key)

	case *Value:
		return w.writeValue(node)

	}
	return next()
}

func (w *jsonVisitor) writeValue(node *Value) error {
	switch {
	case node.Bool != nil:
		fmt.Fprintf(w, "%v", *node.Bool)

	case node.Number != nil:
		fmt.Fprint(w, node.Number.String())

	case node.Str != nil:
		fmt.Fprintf(w, "%q", *node.Str)

	case node.HaveList:
		fmt.Fprint(w, "[")
		for i, e := range node.List {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			if err := w.writeValue(e); err != nil {
				return err
			}
		}
		fmt.Fprint(w, "]")

	case node.HaveMap:
		fmt.Fprint(w, "{")
		for i, e := range node.Map {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			if err := w.writeValue(e.Key); err != nil {
				return err
			}
			fmt.Fprint(w, ":")
			if err := w.writeValue(e.Value); err != nil {
				return err
			}
		}
		fmt.Fprint(w, "}")

	case node.Type != nil:
		fmt.Fprintf(w, "%q", *node.Type)

	default:
		panic(repr.String(node, repr.Hide(lexer.Position{})))
	}
	return nil
}
