package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func TestMain(m *testing.M) {
	err := os.MkdirAll("./tests/cache", 0o700)
	if err != nil {
		panic(err)
	}

	fakeServerDir("./tests/from-env")
	fakeServerDir("./tests/validate")
	fakeServerDir("./tests/generate")
	fakeServerDir("./tests/generate-json")
	fakeServerDir("./tests/generate-yaml")
	fakeServerDir("./tests/load-json")
	fakeServerDir("./tests/load-yaml")
	fakeServerDir("./tests/load-both")

	print.SetVerbose()

	os.Exit(m.Run())
}

func fakeServerDir(path string) {
	_ = os.MkdirAll(filepath.Join(path, "gamemodes"), 0o700)
	_ = os.MkdirAll(filepath.Join(path, "filterscripts"), 0o700)
	_ = os.MkdirAll(filepath.Join(path, "plugins"), 0o700)
	f, _ := os.Create(filepath.Join(path, "gamemodes", "rivershell.amx"))
	f.Close() // nolint
	f, _ = os.Create(filepath.Join(path, "filterscripts", "admin.amx"))
	f.Close() // nolint
	f, _ = os.Create(filepath.Join(path, "plugins", "mysql.amx"))
	f.Close() // nolint
}
