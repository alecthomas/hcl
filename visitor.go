package hcl

import (
	"fmt"
)

// Visit nodes in the AST.
//
// "next" may be called to continue traversal of child nodes.
func Visit(node Node, visit func(node Node, next func() error) error) error {
	return visit(node, func() error {
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
			case node.HaveList:
				for _, entry := range node.List {
					if err := Visit(entry, visit); err != nil {
						return err
					}
				}
			case node.HaveMap:
				for _, entry := range node.Map {
					if err := Visit(entry, visit); err != nil {
						return err
					}
				}
			}

		case *Entry:
			if node.Attribute != nil {
				return Visit(node.Attribute, visit)
			}
			return Visit(node.Block, visit)

		default:
			panic(fmt.Sprintf("%T", node))
		}
		return nil
	})
}
