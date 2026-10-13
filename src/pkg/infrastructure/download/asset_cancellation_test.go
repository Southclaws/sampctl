package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
)

type cancelAtEOFBody struct {
	cancel context.CancelFunc
}

func (body cancelAtEOFBody) Read(data []byte) (int, error) {
	body.cancel()
	return copy(data, "replacement"), io.EOF
}

func (cancelAtEOFBody) Close() error { return nil }

func TestReleaseAssetCancellationPreservesExistingCache(t *testing.T) {
	for _, beforeTransfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "during body", true: "before transfer"}[beforeTransfer], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			path := filepath.Join(t.TempDir(), "asset")
			if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
				t.Fatal(err)
			}
			api := &fakeReleasesAPI{downloadRC: cancelAtEOFBody{cancel: cancel}}
			if beforeTransfer {
				cancel()
			}
			_, err := downloadReleaseAsset(ReleaseAssetDownloadRequest{
				Context: ctx, Client: api, Asset: &github.ReleaseAsset{ID: github.Int64(1)}, Destination: path,
			})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("downloadReleaseAsset() = %v, want cancellation", err)
			}
			if beforeTransfer && api.downloadCalls != 0 {
				t.Errorf("download calls = %d, want none after cancellation", api.downloadCalls)
			}
			assertCanceledAssetCache(t, path)
		})
	}
}

func TestHTTPDownloadCancellationAtEOFPreservesExistingCache(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path := filepath.Join(t.TempDir(), "asset")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusOK, "")
		response.Body = cancelAtEOFBody{cancel: cancel}
		return response, nil
	})
	_, err := FromNetWithClient(ctx, client, "https://example.invalid/asset", path)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("FromNetWithClient() = %v, want cancellation", err)
	}
	assertCanceledAssetCache(t, path)
}

func assertCanceledAssetCache(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "previous" {
		t.Errorf("ReadFile(cache) = %q, %v, want preserved content", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadDir(cache) = %v, %v, want no temporary files", entries, err)
	}
}
