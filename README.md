# Parsing, encoding and decoding of HCL to and from Go types
[![](https://godoc.org/github.com/alecthomas/hcl?status.svg)](http://godoc.org/github.com/alecthomas/hcl) [![CircleCI](https://img.shields.io/circleci/project/github/alecthomas/hcl.svg)](https://circleci.com/gh/alecthomas/hcl) [![Go Report Card](https://goreportcard.com/badge/github.com/alecthomas/hcl)](https://goreportcard.com/report/github.com/alecthomas/hcl) [![Slack chat](https://img.shields.io/static/v1?logo=slack&style=flat&label=slack&color=green&message=gophers)](https://gophers.slack.com/messages/CN9DS8YF3)

This package provides idiomatic Go functions for marshalling and unmarshalling HCL, as well
as an AST 

It supports the same tags as the Hashicorp [hcl2](https://github.com/hashicorp/hcl/tree/hcl2) 
`gohcl` package, but is much less complex.

Unlike `gohcl` it also natively supports `time.Duration`, `time.Time`, `encoding.TextUnmarshaler`
and `json.Unmarshaler`.

It is HCL1 compatible and does not support any HCL2 specific features.

## Design

HCL -> AST -> Go -> AST -> HCL

Mapping can start from any point in this cycle.

Marshalling, unmarshalling, parsing and serialisation are all structurally
isomorphic operations. That is, HCL can be deserialised into an AST or Go, 
or vice versa, and the structure on both ends will be identical.

HCL is always parsed into an AST before unmarshaling and, similarly, Go structures
are always mapped to an AST before being serialised to HCL.

Between          | And          | Preserves
-----------------|--------------|-----------------
HCL              | AST          | Structure, values, order, comments.
HCL              | Go           | Structure, values, partial comments (via the `help:""` tag).
AST              | Go           | Structure, values.

## Schema reflection

HCL has no real concept of schemas (that I can find), but there is precedent for something similar
in Terraform variable definition files. This package supports reflecting a rudimentary schema from Go,
where the value for each attribute is one of the scalar types `number`, `string` or `boolean`. 
Lists and maps are typed by example.

Here's an example schema.

```
// A string field.
str = string
num = number
bool = boolean
list = [string]

// A map.
map = {
  string: number,
}

// A block.
block "name" {
  attr = string
}

// Repeated blocks.
block_slice "label0" "label1" {
  attr = string
}
```

Comments are from `help:""` tags. See [schema_test.go](https://github.com/alecthomas/hcl/blob/master/schema_test.go) for details.


## Struct field tags

The tag format is as with other similar serialisation packages:

```
hcl:"[<name>][,<option>]"
```

The supported options are:

Tag                  | Description
---------------------|--------------------------------------
`attr` (default)     | Specifies that the value is to be populated from an attribute.
`block`              | Specifies that the value is to populated from a block.
`label`              | Specifies that the value is to populated from a block label.
`optional`           | As with attr, but the field is optional.
`remain`             | Specifies that the value is to be populated from the remaining body after populating other fields. The field must be of type `[]*hcl.Entry`.

Additionally, a separate `help:""` tag can be specified to populate
comment fields in the AST when serialising Go structures.
