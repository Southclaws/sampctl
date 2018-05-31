package compiler

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"

	"github.com/Southclaws/sampctl/print"
)

var gh *github.Client

func TestMain(m *testing.M) {
	godotenv.Load("../.env", "../../.env") //nolint
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})))

	print.SetVerbose()

	os.Exit(m.Run())
}
