package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	res "github.com/Southclaws/sampctl/src/pkg/package/resource"
)

func TestEnsureVersionedPluginCachedUsesDefinitionOnAssetMiss(t *testing.T) {
	t.Parallel()
	for _, scheme := range []string{"", "plugin", "component"} {
		t.Run(fmt.Sprintf("scheme=%q", scheme), func(t *testing.T) {
			cacheDir := filepath.Join(t.TempDir(), "cache")
			meta := versioning.DependencyMeta{User: "fixture", Repo: "legacy", Tag: "v1.0.0", Scheme: scheme}
			definition := res.Resource{Name: `^legacy\.so$`, Platform: "linux", Plugins: []string{"legacy.so"}}
			assetPath := seedCachedSinglePluginAsset(t, cacheDir, meta, definition, "legacy.so")
			if err := os.Remove(assetPath); err != nil {
				t.Fatal(err)
			}
			var releaseRequests, assetRequests, definitionRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/fixture/legacy/releases/tags/v1.0.0":
					releaseRequests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					release := github.RepositoryRelease{
						TagName: github.String("v1.0.0"),
						Assets:  []github.ReleaseAsset{{ID: github.Int64(7), Name: github.String("legacy.so")}},
					}
					if err := json.NewEncoder(w).Encode(release); err != nil {
						t.Errorf("release response encode error = %v", err)
					}
				case "/repos/fixture/legacy/releases/assets/7":
					assetRequests.Add(1)
					w.Header().Set("Content-Type", "application/octet-stream")
					if _, err := w.Write([]byte("legacy plugin")); err != nil {
						t.Errorf("plugin response write error = %v", err)
					}
				default:
					definitionRequests.Add(1)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			client := github.NewClient(server.Client())
			baseURL, err := url.Parse(server.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			client.BaseURL = baseURL
			request := EnsureVersionedPluginCachedRequest{
				Context: context.Background(), Meta: meta, Platform: "linux", Version: "openmp", CacheDir: cacheDir, GitHub: client,
			}
			filename, resource, err := EnsureVersionedPluginCached(request)
			if err != nil {
				t.Fatalf("EnsureVersionedPluginCached(cached definition, absent asset) error = %v, want release download", err)
			}
			if resource == nil || resource.Name != definition.Name {
				t.Errorf("EnsureVersionedPluginCached(cached definition, absent asset) resource = %v, want cached matcher %s", resource, definition.Name)
			}
			data, err := os.ReadFile(filename)
			if err != nil || string(data) != "legacy plugin" {
				t.Errorf("EnsureVersionedPluginCached(cached definition, absent asset) asset = %q, %v, want downloaded plugin", data, err)
			}
			request.GitHub = nil
			repeatedFilename, _, err := EnsureVersionedPluginCached(request)
			if err != nil || repeatedFilename != filename {
				t.Errorf("EnsureVersionedPluginCached(repeat, no GitHub client) = %q, %v, want cached asset %q", repeatedFilename, err, filename)
			}
			if releases, assets, definitions := releaseRequests.Load(), assetRequests.Load(), definitionRequests.Load(); releases != 1 || assets != 1 || definitions != 0 {
				t.Errorf("EnsureVersionedPluginCached(cached definition) requests = releases:%d assets:%d definitions:%d, want 1, 1, 0", releases, assets, definitions)
			}
		})
	}
}
