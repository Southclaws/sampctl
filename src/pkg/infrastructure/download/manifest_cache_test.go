package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestManifestCacheFreshGet(t *testing.T) {
	for _, manifest := range manifestCacheFixtures() {
		t.Run(manifest.name, func(t *testing.T) {
			for _, scenario := range []string{"valid", "canceled", "malformed"} {
				t.Run(scenario, func(t *testing.T) {
					directory := t.TempDir()
					path := filepath.Join(directory, manifest.filename)
					body := manifest.body
					if scenario == "malformed" {
						body = "{"
					}
					require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
					stderr := captureManifestStderr(t)

					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if scenario == "canceled" {
						cancel()
					}
					calls := 0
					client := roundTripClient(func(*http.Request) (*http.Response, error) {
						calls++
						return nil, errors.New("unexpected manifest request")
					})

					value, err := manifest.get(ctx, directory, client)
					if scenario == "malformed" {
						var syntaxError *json.SyntaxError
						require.ErrorAs(t, err, &syntaxError)
						assert.Equal(t, manifest.zero, value)
					} else {
						require.NoError(t, err)
						assert.Equal(t, manifest.value, value)
					}
					assert.Zero(t, calls)
					assert.Empty(t, stderr())
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, body, string(data))
				})
			}
		})
	}
}

func TestManifestCacheRefresh(t *testing.T) {
	for _, manifest := range manifestCacheFixtures() {
		t.Run(manifest.name, func(t *testing.T) {
			for _, operation := range []string{"get", "update"} {
				t.Run(operation, func(t *testing.T) {
					for _, canceled := range []bool{false, true} {
						t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
							directory := t.TempDir()
							path := filepath.Join(directory, manifest.filename)
							previous := "null\n"
							writeManifestPreviousCache(t, path, previous, operation == "get")
							stderr := captureManifestStderr(t)
							ctx, cancel := context.WithCancel(t.Context())
							defer cancel()
							if canceled {
								cancel()
							}

							body := &manifestCacheBody{reader: strings.NewReader(manifest.body)}
							body.onClose = func() {
								data, err := os.ReadFile(path)
								require.NoError(t, err)
								assert.Equal(t, previous, string(data), "cache changed before response body closed")
								assert.Empty(t, stderr(), "progress preceded response body closure")
							}
							calls := 0
							client := roundTripClient(func(req *http.Request) (*http.Response, error) {
								calls++
								assert.Equal(t, http.MethodGet, req.Method)
								assert.Equal(t, manifest.url, req.URL.String())
								assert.Nil(t, req.Body)
								assert.Empty(t, req.Header)
								assert.Same(t, ctx, req.Context())
								response := jsonResponse(http.StatusOK, "")
								response.Body = body
								return response, nil
							})

							if operation == "get" {
								value, err := manifest.get(ctx, directory, client)
								require.NoError(t, err)
								assert.Equal(t, manifest.value, value)
								assert.Equal(t, manifest.progress, stderr())
							} else {
								require.NoError(t, manifest.update(ctx, directory, client))
								assert.Empty(t, stderr())
							}
							assert.Equal(t, 1, calls)
							assert.Equal(t, 1, body.closes)
							assertManifestCacheValue(t, path, manifest.value)
						})
					}
				})
			}
		})
	}
}

func TestManifestCacheRefreshFailures(t *testing.T) {
	transportFailure := errors.New("transport failed")
	readFailure := errors.New("body read failed")
	closeFailure := errors.New("body close failed")

	for _, manifest := range manifestCacheFixtures() {
		t.Run(manifest.name, func(t *testing.T) {
			for _, operation := range []string{"get", "update"} {
				t.Run(operation, func(t *testing.T) {
					for _, scenario := range []struct {
						name              string
						doError           error
						responseWithError bool
						status            int
						body              string
						readError         error
						wantPrefix        string
						wantCause         error
					}{
						{name: "transport", doError: transportFailure, wantPrefix: "failed to download package list", wantCause: transportFailure},
						{name: "response with transport error", doError: transportFailure, responseWithError: true, wantPrefix: "failed to download package list", wantCause: transportFailure},
						{name: "status", status: http.StatusBadGateway, wantPrefix: manifest.name + " list download returned HTTP 502 Bad Gateway"},
						{name: "decode", body: "{", wantPrefix: "failed to decode package list", wantCause: io.ErrUnexpectedEOF},
						{name: "read", readError: readFailure, wantPrefix: "failed to decode package list", wantCause: readFailure},
						{name: "close", wantPrefix: "failed to close " + manifest.name + " list response body", wantCause: closeFailure},
					} {
						t.Run(scenario.name, func(t *testing.T) {
							directory := t.TempDir()
							path := filepath.Join(directory, manifest.filename)
							previous := "null\n"
							writeManifestPreviousCache(t, path, previous, operation == "get")
							stderr := captureManifestStderr(t)
							payload := scenario.body
							if payload == "" {
								payload = manifest.body
							}
							body := &manifestCacheBody{reader: strings.NewReader(payload), readError: scenario.readError, closeError: closeFailure}
							status := scenario.status
							if status == 0 {
								status = http.StatusOK
							}
							calls := 0
							client := roundTripClient(func(*http.Request) (*http.Response, error) {
								calls++
								if scenario.doError != nil && !scenario.responseWithError {
									return nil, scenario.doError
								}
								response := jsonResponse(status, "")
								response.Body = body
								return response, scenario.doError
							})

							var err error
							if operation == "get" {
								var value any
								value, err = manifest.get(t.Context(), directory, client)
								assert.Equal(t, manifest.zero, value)
							} else {
								err = manifest.update(t.Context(), directory, client)
							}
							require.Error(t, err)
							assert.True(t, strings.HasPrefix(err.Error(), scenario.wantPrefix), "error = %v, want prefix %q", err, scenario.wantPrefix)
							if scenario.wantCause != nil {
								assert.ErrorIs(t, err, scenario.wantCause)
							}
							if scenario.wantCause != closeFailure {
								assert.NotErrorIs(t, err, closeFailure)
							}
							if scenario.doError != nil {
								assert.Zero(t, body.closes)
							} else {
								assert.Equal(t, 1, body.closes)
							}
							if scenario.doError != nil || status != http.StatusOK {
								assert.Zero(t, body.reads)
							}
							assert.Equal(t, 1, calls)
							assert.Empty(t, stderr())
							data, readErr := os.ReadFile(path)
							require.NoError(t, readErr)
							assert.Equal(t, previous, string(data))
						})
					}
				})
			}
		})
	}
}

func TestManifestCacheNullAndEmptyResults(t *testing.T) {
	for _, manifest := range manifestCacheFixtures() {
		t.Run(manifest.name, func(t *testing.T) {
			for _, operation := range []string{"get", "update"} {
				t.Run(operation, func(t *testing.T) {
					for _, result := range []struct {
						name  string
						body  string
						value any
					}{
						{name: "null", body: "null", value: manifest.zero},
						{name: "empty", body: manifest.emptyBody, value: manifest.empty},
					} {
						t.Run(result.name, func(t *testing.T) {
							directory := t.TempDir()
							stderr := captureManifestStderr(t)
							client := roundTripClient(func(*http.Request) (*http.Response, error) {
								return jsonResponse(http.StatusOK, result.body), nil
							})

							if operation == "get" {
								value, err := manifest.get(t.Context(), directory, client)
								require.NoError(t, err)
								assert.Equal(t, result.value, value)
								assert.Equal(t, manifest.progress, stderr())
							} else {
								require.NoError(t, manifest.update(t.Context(), directory, client))
								assert.Empty(t, stderr())
							}
							assertManifestCacheValue(t, filepath.Join(directory, manifest.filename), result.value)
						})
					}
				})
			}
		})
	}
}

func TestManifestCachePublicationFailure(t *testing.T) {
	for _, manifest := range manifestCacheFixtures() {
		t.Run(manifest.name, func(t *testing.T) {
			for _, operation := range []string{"get", "update"} {
				t.Run(operation, func(t *testing.T) {
					directory := filepath.Join(t.TempDir(), "cache")
					require.NoError(t, os.WriteFile(directory, []byte("keep"), 0o600))
					stderr := captureManifestStderr(t)
					body := &manifestCacheBody{reader: strings.NewReader(manifest.body)}
					client := roundTripClient(func(*http.Request) (*http.Response, error) {
						response := jsonResponse(http.StatusOK, "")
						response.Body = body
						return response, nil
					})

					var err error
					if operation == "get" {
						var value any
						value, err = manifest.get(t.Context(), directory, client)
						assert.Equal(t, manifest.zero, value)
					} else {
						err = manifest.update(t.Context(), directory, client)
					}
					var pathError *os.PathError
					require.ErrorAs(t, err, &pathError)
					assert.Same(t, pathError, err)
					assert.Equal(t, directory, pathError.Path)
					assert.Equal(t, 1, body.closes)
					assert.Empty(t, stderr())
					data, readErr := os.ReadFile(directory)
					require.NoError(t, readErr)
					assert.Equal(t, "keep", string(data))
				})
			}
		})
	}
}

type manifestCacheFixture struct {
	name      string
	filename  string
	url       string
	body      string
	progress  string
	value     any
	zero      any
	emptyBody string
	empty     any
	get       func(context.Context, string, HTTPDoer) (any, error)
	update    func(context.Context, string, HTTPDoer) error
}

func manifestCacheFixtures() []manifestCacheFixture {
	return []manifestCacheFixture{
		{
			name: "package", filename: "packages.json", url: "http://list.packages.sampctl.com",
			body: `[{"repo":"repo","user":"user"}]`, progress: "updating package list...\n",
			value: []pawnpackage.Package{{Repo: "repo", User: "user"}}, zero: []pawnpackage.Package(nil),
			emptyBody: `[]`, empty: []pawnpackage.Package{},
			get: func(ctx context.Context, directory string, client HTTPDoer) (any, error) {
				return GetPackageListWithClientContext(ctx, directory, client)
			},
			update: UpdatePackageListWithClientContext,
		},
		{
			name: "runtime", filename: "runtimes.json", url: "https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json",
			body: `{"aliases":{"latest":"0.3.7"},"packages":[{"version":"0.3.7"}]}`, progress: "updating runtimes list...\n",
			value: Runtimes{Aliases: map[string]string{"latest": "0.3.7"}, Packages: []RuntimePackage{{Version: "0.3.7"}}}, zero: Runtimes{},
			emptyBody: `{"aliases":{},"packages":[]}`, empty: Runtimes{Aliases: map[string]string{}, Packages: []RuntimePackage{}},
			get: func(ctx context.Context, directory string, client HTTPDoer) (any, error) {
				return GetRuntimeListWithClientContext(ctx, directory, client)
			},
			update: UpdateRuntimeListWithClientContext,
		},
	}
}

func writeManifestPreviousCache(t *testing.T, path, body string, expired bool) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	if expired {
		old := time.Now().Add(-8 * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, old, old))
	}
}

func assertManifestCacheValue(t *testing.T, path string, want any) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	expected, err := json.Marshal(want)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(data))
}

func captureManifestStderr(t *testing.T) func() string {
	t.Helper()

	output, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	previous := os.Stderr
	os.Stderr = output
	t.Cleanup(func() {
		os.Stderr = previous
		assert.NoError(t, output.Close())
	})

	return func() string {
		data, err := os.ReadFile(output.Name())
		require.NoError(t, err)
		return string(data)
	}
}

type manifestCacheBody struct {
	reader     io.Reader
	readError  error
	closeError error
	reads      int
	closes     int
	onClose    func()
}

func (body *manifestCacheBody) Read(data []byte) (int, error) {
	body.reads++
	if body.readError != nil {
		return 0, body.readError
	}

	return body.reader.Read(data)
}

func (body *manifestCacheBody) Close() error {
	body.closes++
	if body.onClose != nil {
		body.onClose()
	}

	return body.closeError
}
