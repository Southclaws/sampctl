package compiler

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

var gh *github.Client

func TestMain(m *testing.M) {
	err := godotenv.Load("../.env", "../../.env")
	if err != nil {
		panic(err)
	}
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")})))

	os.Exit(m.Run())
}
