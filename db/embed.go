// Package db embeds the SQL migrations and the shipped marketplace definitions so the API binary
// can apply them on start-up.
package db

import "embed"

// Migrations holds the goose migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Marketplaces holds the shipped marketplace definitions (declarative "rules" adapters), one JSON
// file per marketplace. They are upserted into source_adapters at start-up.
//
//go:embed marketplaces/*.json
var Marketplaces embed.FS
