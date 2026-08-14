package migrations

import "embed"

// Files contains every committed SQL migration shipped with this binary.
//
//go:embed *.sql
var Files embed.FS
