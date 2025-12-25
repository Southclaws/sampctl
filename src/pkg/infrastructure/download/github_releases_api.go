package download

import (
	"context"

	"github.com/google/go-github/github"
)

// GitHubReleasesAPI wrap the go-github api for release lookups
type GitHubReleasesAPI interface {
	ListReleases(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error)
}

type githubClientReleasesAdapter struct {
	client *github.Client
}

func (a githubClientReleasesAdapter) ListReleases(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error) {
	return a.client.Repositories.ListReleases(ctx, owner, repo, opt)
}

func (a githubClientReleasesAdapter) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error) {
	return a.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
}
