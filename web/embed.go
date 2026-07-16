package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html assets/*
var files embed.FS

func Files() fs.FS {
	return files
}

func Assets() fs.FS {
	assets, err := fs.Sub(files, "assets")
	if err != nil {
		panic(err)
	}
	return assets
}
