package rook

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

var gh *github.Client
var gitAuth transport.AuthMethod

func TestMain(m *testing.M) {
	_ = godotenv.Load("../.env", "../../.env")

	token := os.Getenv("FULL_ACCESS_GITHUB_TOKEN")
	if len(token) == 0 {
		fmt.Println("No token in `FULL_ACCESS_GITHUB_TOKEN`, skipping tests.")
		return
	}
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})))

	err := os.MkdirAll("./tests/cache", 0700)
	if err != nil {
		panic(err)
	}

	print.SetVerbose()

	os.Exit(m.Run())
}
