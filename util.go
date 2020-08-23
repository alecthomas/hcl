package hcl

import (
	"fmt"
	"strings"
	"unicode"
)

// StripComments recursively from an AST node.
func StripComments(node Node) error {
	return Visit(node, func(node Node, next func() error) error {
		switch node := node.(type) {
		case *Attribute:
			node.Comments = nil

		case *Block:
			node.Comments = nil

		case *MapEntry:
			node.Comments = nil
		}
		return next()
	})
}

// AddParentRefs recursively updates an AST's parent references.
//
// This is called automatically during Parse*(), but can be called on a manually constructed AST.
func AddParentRefs(node Node) error {
	addParentRefs(nil, node)
	return nil
}

func addParentRefs(parent, node Node) {
	switch node := node.(type) {
	case *AST:
		for _, entry := range node.Entries {
			addParentRefs(node, entry)
		}

	case *Block:
		node.Parent = parent
		for _, entry := range node.Body {
			addParentRefs(node, entry)
		}

	case *Entry:
		node.Parent = parent
		if node.Attribute != nil {
			addParentRefs(node, node.Attribute)
		} else {
			addParentRefs(node, node.Block)
		}

	case *MapEntry:
		node.Parent = parent

	case *Value:
		node.Parent = parent
		switch {
		case node.HaveList:
			for _, entry := range node.List {
				addParentRefs(node, entry)
			}
		case node.HaveMap:
			for _, entry := range node.Map {
				addParentRefs(node, entry)
			}
		}

	case *Attribute:
		node.Parent = parent
		addParentRefs(node, node.Value)

	default:
		panic(fmt.Sprintf("%T", node))
	}
}

func dedent(s string) string {
	lines := strings.Split(s, "\n")
	indent := whitespacePrefix(lines[0])
	for _, line := range lines {
		candidate := whitespacePrefix(line)
		if len(candidate) < len(indent) {
			indent = candidate
		}
	}
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, indent)
	}
	return strings.Join(lines, "\n")
}

func whitespacePrefix(s string) string {
	indent := ""
	for _, rn := range s {
		if unicode.IsSpace(rn) {
			indent += string(rn)
		}
	}
	return indent
}
