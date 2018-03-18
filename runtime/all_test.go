package runtime

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"

	"github.com/Southclaws/sampctl/print"
)

var gh *github.Client
var Version = ""

func TestMain(m *testing.M) {
	godotenv.Load("../.env", "../../.env")
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})))

	v, err := ioutil.ReadFile("../VERSION")
	if err != nil {
		panic(err)
	}
	Version = string(v)

	err = os.MkdirAll("./tests/cache", 0700)
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
