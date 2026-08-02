// Package database owns the PostgreSQL connection pool and applies the schema
// migrations at startup.
//
// Domain packages depend on this; it depends on none of them, so their queries
// live with them rather than here.
package database
