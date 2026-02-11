package migrations

import "embed"

// FS contains versioned SQL migrations bundled into the binary.
//
//go:embed *.sql
var FS embed.FS
