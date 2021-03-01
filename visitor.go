package hcl

import (
	"reflect"
)

// Visit nodes in the AST.
//
// "next" may be called to continue traversal of child nodes.
func Visit(node Node, visitor func(node Node, next func() error) error) error {
	if reflect.ValueOf(node).IsNil() { // Workaround for Go's typed nil interfaces.
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
