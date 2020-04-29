package hcl

// StripComments recursively from an AST node.
func StripComments(node Node) error {
	return Visit(node, func(node Node) error {
		switch node := node.(type) {
		case *Entry:
			node.Comments = nil

		case *MapEntry:
			node.Comments = nil

		case *Value:
			node.Comments = nil
		}
		return nil
	})
}
