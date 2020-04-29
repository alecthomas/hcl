package hcl

import (
	"fmt"
)

// Visit nodes in the AST.
func Visit(node Node, visit func(node Node) error) error {
	if err := visit(node); err != nil {
		return err
	}
	switch node := node.(type) {
	case *AST:
		for _, entry := range node.Entries {
			if err := Visit(entry, visit); err != nil {
				return err
			}
		}
	case *MapEntry:
		if err := Visit(node.Value, visit); err != nil {
			return err
		}
	case *Attribute:
		if err := Visit(node.Value, visit); err != nil {
			return err
		}
	case *Block:
		for _, entry := range node.Body {
			if err := Visit(entry, visit); err != nil {
				return err
			}
		}
	case *Value:
		switch {
		case node.List != nil:
			for _, entry := range node.List {
				if err := Visit(entry, visit); err != nil {
					return err
				}
			}
		case node.Map != nil:
			for _, entry := range node.Map {
				if err := Visit(entry, visit); err != nil {
					return err
				}
			}
		}
	default:
		panic(fmt.Sprintf("%T", node))
	}
	return nil
}
