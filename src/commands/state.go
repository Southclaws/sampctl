package commands

import (
	"io"
	"net/http"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
)

const commandStateKey = "commandState"

type commandState struct {
	cacheDir string
	version  string
	cfg      *config.Config
	gh       *github.Client
	gitAuth  transport.AuthMethod
}

func newCommandState(version, cacheDir string) *commandState {
	return &commandState{
		cacheDir: cacheDir,
		version:  version,
	}
}

func newGitHubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}

	client := &http.Client{
		Transport: &githubPublicReadFallbackTransport{
			authed: &oauth2.Transport{
				Source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
				Base:   http.DefaultTransport,
			},
			unauth: http.DefaultTransport,
		},
	}
	return github.NewClient(client)
}

type githubPublicReadFallbackTransport struct {
	authed http.RoundTripper
	unauth http.RoundTripper
}

func (t *githubPublicReadFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.authedTransport().RoundTrip(req)
	if err != nil || req == nil || req.Method != http.MethodGet || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	if err = closeHTTPResponseBody(resp); err != nil {
		return nil, err
	}

	fallbackReq := req.Clone(req.Context())
	fallbackReq.Header = req.Header.Clone()
	fallbackReq.Header.Del("Authorization")

	return t.unauthTransport().RoundTrip(fallbackReq)
}

func (t *githubPublicReadFallbackTransport) authedTransport() http.RoundTripper {
	if t == nil || t.authed == nil {
		return http.DefaultTransport
	}
	return t.authed
}

func (t *githubPublicReadFallbackTransport) unauthTransport() http.RoundTripper {
	if t == nil || t.unauth == nil {
		return http.DefaultTransport
	}
	return t.unauth
}

func closeHTTPResponseBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return errors.Wrapf(closeErr, "failed to close response body after drain error: %v", err)
		}
		return errors.Wrap(err, "failed to drain response body")
	}

	if err := resp.Body.Close(); err != nil {
		return errors.Wrap(err, "failed to close response body")
	}

	return nil
}

func getCommandState(c *cli.Context) (*commandState, error) {
	if c == nil || c.App == nil {
		return nil, errors.New("command context is not available")
	}

	state, ok := c.App.Metadata[commandStateKey].(*commandState)
	if !ok || state == nil {
		return nil, errors.New("command state is not available")
	}

	return state, nil
}
