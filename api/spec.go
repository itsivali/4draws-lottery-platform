// Package spec carries the OpenAPI document (openapi.yaml) inside the binary, so the API can serve its own contract
// and describe itself on its home page.
package spec

import _ "embed"

// OpenAPI is api/openapi.yaml as written: the contract the router is tested against.
//
//go:embed openapi.yaml
var OpenAPI []byte
