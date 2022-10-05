package hcl

import (
	"reflect"
)

// Visit nodes in the AST.
//
// "next" may be called to continue traversal of child nodes.
func Visit(node Node, visitor func(node Node, next func() error) error) error {
	if node == nil || reflect.ValueOf(node).IsNil() { // Workaround for Go's typed nil interfaces.
		return nil
	}
	return visitor(node, func() error {
		for _, child := range node.children() {
			if err := Visit(child, visitor); err != nil {
				return err
			}
		}
		return nil
	})
}

// Find and return blocks, attributes or map entries with the given key.
func Find(node Node, names ...string) (nodes []Node) {
	_ = Visit(node, func(node Node, next func() error) error {
		switch node := node.(type) {
		case *Block:
			for _, name := range names {
				if node.Name == name {
					nodes = append(nodes, node)
					break
				}
			}

		case *Attribute:
			for _, name := range names {
				if node.Key == name {
					nodes = append(nodes, node)
					break
				}
			}

		case *MapEntry:
			for _, name := range names {
				if str, ok := node.Key.(*String); ok && str.Str == name {
					nodes = append(nodes, node)
					break
				}
			}
		}
		return next()
	})
	return
}
