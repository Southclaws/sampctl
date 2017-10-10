package main

import (
	"bytes"
	"html/template"
	"net/url"
	"path/filepath"
)

var (
	pawnMacOS = "https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-darwin.zip"
	pawnLinux = "https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-linux.tar.gz"
	pawnWin32 = "https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-windows.zip"
)

func compilerURL(rawurl, version string) (download, filename string) {
	tmpl := template.Must(template.New("tmp").Parse(rawurl))
	wr := &bytes.Buffer{}
	err := tmpl.Execute(wr, struct{ Version string }{version})
	if err != nil {
		panic(err)
	}

	download = wr.String()
	u, err := url.Parse(download)
	if err != nil {
		return
	}
	filename = filepath.Base(u.Path)

	return
}
