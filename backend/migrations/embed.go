// Package migrations carries the SQL schema migrations as embedded files, so
// the binary can apply them without the .sql files being deployed beside it.
package migrations

import "embed"

// FS holds every migration, named <version>_<description>.sql. The numeric
// prefix orders them and is the version recorded once applied.
//
//go:embed *.sql
var FS embed.FS
