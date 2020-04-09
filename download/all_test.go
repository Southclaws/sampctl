package download

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

var gh *github.Client

func TestMain(m *testing.M) {
	godotenv.Load("../.env", "../../.env")

	token := os.Getenv("GITHUB_TOKEN_FULL_ACCESS")
	if len(token) == 0 {
		fmt.Println("No token in `GITHUB_TOKEN_FULL_ACCESS`, skipping tests.")
		return
	}
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})))

	os.Exit(m.Run())
}
