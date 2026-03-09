package pkgcontext

import (
	"os"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
)

var (
	gh      *github.Client
	gitAuth transport.AuthMethod
)

func TestMain(m *testing.M) {
	err := os.MkdirAll("./tests/cache", 0o700)
	if err != nil {
		panic(err)
	}

	print.SetVerbose()

	os.Exit(m.Run())
}
