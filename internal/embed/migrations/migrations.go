package migrations

import (
	"embed"
	"io/fs"
)

//go:embed sql/*.sql
var migrationsFS embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(migrationsFS, "sql")
	if err != nil {
		panic(err)
	}
	return sub
}
