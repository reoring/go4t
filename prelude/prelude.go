// Package prelude contains the gofx definitions included in every program.
package prelude

import _ "embed"

// Source is parsed and checked before each user file.
//
//go:embed prelude.gfx
var Source string
