// Package db embeds the SQL migrations so the API binary can apply them on start-up.
package db

import "embed"

// Migrations holds the goose migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS
