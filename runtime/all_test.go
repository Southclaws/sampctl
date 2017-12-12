package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	fakeServerDir("./tests/from-env")
	fakeServerDir("./tests/validate")
	fakeServerDir("./tests/generate")
	fakeServerDir("./tests/generate-json")
	fakeServerDir("./tests/generate-yaml")
	fakeServerDir("./tests/load-json")
	fakeServerDir("./tests/load-yaml")
	fakeServerDir("./tests/load-both")

	os.Exit(m.Run())
}

func fakeServerDir(path string) {
	os.MkdirAll(filepath.Join(path, "gamemodes"), 0755)
	os.MkdirAll(filepath.Join(path, "filterscripts"), 0755)
	os.MkdirAll(filepath.Join(path, "plugins"), 0755)
	f, _ := os.Create(filepath.Join(path, "gamemodes", "rivershell.amx"))
	f.Close() // nolint
	f, _ = os.Create(filepath.Join(path, "filterscripts", "admin.amx"))
	f.Close() // nolint
	f, _ = os.Create(filepath.Join(path, "plugins", "mysql.amx"))
	f.Close() // nolint
}
