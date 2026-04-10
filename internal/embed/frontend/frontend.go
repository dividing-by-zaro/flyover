package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var frontendFS embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
