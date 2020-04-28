# Parsing, encoding and decoding of HCL to and from Go types
[![](https://godoc.org/github.com/alecthomas/hcl?status.svg)](http://godoc.org/github.com/alecthomas/hcl) [![CircleCI](https://img.shields.io/circleci/project/github/alecthomas/hcl.svg)](https://circleci.com/gh/alecthomas/hcl) [![Go Report Card](https://goreportcard.com/badge/github.com/alecthomas/hcl)](https://goreportcard.com/report/github.com/alecthomas/hcl) [![Slack chat](https://img.shields.io/static/v1?logo=slack&style=flat&label=slack&color=green&message=gophers)](https://gophers.slack.com/messages/CN9DS8YF3)

This package provides idiomatic Go functions for marshalling and unmarshalling HCL, as well
as an AST parser.

It supports the same tags as the Hashicorp [hcl2](https://github.com/hashicorp/hcl/tree/hcl2) 
`gohcl` package, but is much less complex.

Unlike `gohcl` it also natively supports `time.Duration`, `time.Time`, `encoding.TextUnmarshaler`
and `json.Unmarshaler`.

It is HCL1 compatible and does not support any HCL2 specific features.

