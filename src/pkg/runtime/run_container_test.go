package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type fakeContainerRemover struct {
	called      bool
	containerID string
	options     client.ContainerRemoveOptions
}

func (f *fakeContainerRemover) ContainerRemove(_ context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.called = true
	f.containerID = containerID
	f.options = options

	return client.ContainerRemoveResult{}, nil
}

func TestRemoveContainerUsesForce(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerRemover{}
	err := removeContainer(context.Background(), cli, "container-id")
	require.NoError(t, err)
	require.True(t, cli.called)
	require.Equal(t, "container-id", cli.containerID)
	require.True(t, cli.options.Force)
}

func TestRemoveContainerSkipsEmptyID(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerRemover{}
	err := removeContainer(context.Background(), cli, "")
	require.NoError(t, err)
	require.False(t, cli.called)
}

type fakeContainerWaiter struct {
	response container.WaitResponse
	err      error
}

func (f fakeContainerWaiter) ContainerWait(_ context.Context, _ string, _ client.ContainerWaitOptions) client.ContainerWaitResult {
	responseCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)

	if f.err != nil {
		errCh <- f.err
	} else {
		responseCh <- f.response
	}

	return client.ContainerWaitResult{Result: responseCh, Error: errCh}
}

func TestWaitForContainerExitAllowsZeroStatus(t *testing.T) {
	t.Parallel()

	err := waitForContainerExit(context.Background(), fakeContainerWaiter{
		response: container.WaitResponse{StatusCode: 0},
	}, "container-id")
	require.NoError(t, err)
}

func TestWaitForContainerExitRejectsNonZeroStatus(t *testing.T) {
	t.Parallel()

	err := waitForContainerExit(context.Background(), fakeContainerWaiter{
		response: container.WaitResponse{
			StatusCode: 137,
			Error:      &container.WaitExitError{Message: "terminated"},
		},
	}, "container-id")
	require.EqualError(t, err, "container exited with status code 137: terminated")
}

type fakeContainerStopper struct {
	killCalled    bool
	killSignal    string
	killErr       error
	waitResponse  container.WaitResponse
	waitErr       error
	waitDelay     time.Duration
	waitCondition container.WaitCondition
}

func (f *fakeContainerStopper) ContainerKill(_ context.Context, _ string, options client.ContainerKillOptions) (client.ContainerKillResult, error) {
	f.killCalled = true
	f.killSignal = options.Signal

	return client.ContainerKillResult{}, f.killErr
}

func (f *fakeContainerStopper) ContainerWait(ctx context.Context, _ string, options client.ContainerWaitOptions) client.ContainerWaitResult {
	f.waitCondition = options.Condition
	responseCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)

	go func() {
		if f.waitDelay > 0 {
			select {
			case <-time.After(f.waitDelay):
			case <-ctx.Done():
				return
			}
		}

		if f.waitErr != nil {
			errCh <- f.waitErr
			return
		}

		responseCh <- f.waitResponse
	}()

	return client.ContainerWaitResult{Result: responseCh, Error: errCh}
}

func TestStopContainerSendsSIGINTAndWaits(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerStopper{waitResponse: container.WaitResponse{StatusCode: 0}}
	err := stopContainer(context.Background(), cli, "container-id")
	require.NoError(t, err)
	require.True(t, cli.killCalled)
	require.Equal(t, "SIGINT", cli.killSignal)
	require.Equal(t, container.WaitConditionNotRunning, cli.waitCondition)
}

func TestStopContainerReturnsWaitError(t *testing.T) {
	t.Parallel()

	cli := &fakeContainerStopper{waitErr: errors.New("wait failed")}
	err := stopContainer(context.Background(), cli, "container-id")
	require.EqualError(t, err, "wait failed")
	require.True(t, cli.killCalled)
}

func TestRunContainerNegotiatesDocker25AndPullsMissingImage(t *testing.T) {
	testRunContainer(t, RunOptions{}, nil)
}

func TestRunContainerUsesExplicitArgumentsAndProjectDirectory(t *testing.T) {
	testRunContainer(t, RunOptions{
		ProjectDir: t.TempDir(),
		Args:       []string{"--dir", "/samp", "--build", "release", "--forceBuild", "development"},
	}, []string{"--dir", "/samp", "--build", "release", "--forceBuild", "development"})
}

func TestRunContainerArgumentCompatibility(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"embedding-program", "server", "run", "--container", "--mountCache", "--build", "legacy build", "", "$literal", `"quoted"`, "semi;colon", "development"}
	legacy := []string{"--build", "legacy build", "", "$literal", `"quoted"`, "semi;colon", "development"}
	explicit := []string{"--build", "explicit build", "", "$explicit", `"quoted"`, "semi;colon"}

	for _, test := range []struct {
		name     string
		passArgs bool
		args     []string
		want     []string
	}{
		{name: "disabled nil"},
		{name: "legacy nil", passArgs: true, want: legacy},
		{name: "explicit", args: explicit, want: explicit},
		{name: "explicit overrides legacy", passArgs: true, args: explicit, want: explicit},
		{name: "explicit empty", args: []string{}},
		{name: "explicit empty overrides legacy", passArgs: true, args: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			argsBefore := append([]string(nil), os.Args...)
			var storage []string
			if test.args != nil {
				storage = append(append([]string{}, test.args...), "unused capacity")
				test.args = storage[:len(test.args)]
			}
			storageBefore := append([]string(nil), storage...)

			testRunContainer(t, RunOptions{PassArgs: test.passArgs, Args: test.args}, test.want)

			require.Equal(t, argsBefore, os.Args)
			require.Equal(t, storageBefore, storage)
		})
	}

	os.Args = []string{"embedding-program"}
	testRunContainer(t, RunOptions{PassArgs: true}, nil)
}

func testRunContainer(t *testing.T, options RunOptions, wantArgs []string) {
	t.Helper()

	workingDir := t.TempDir()
	cacheDir := t.TempDir()
	projectDir := options.ProjectDir
	if projectDir == "" {
		projectDir = workingDir
	}
	port := 8192
	image := "ghcr.io/southclaws/sampctl:test-version"
	var mu sync.Mutex
	var requests []string
	createCalls := 0

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.Method + " " + r.URL.Path {
		case "HEAD /_ping":
			w.Header().Set("API-Version", "1.44")
		case "POST /v1.44/containers/create":
			var request container.CreateRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode ContainerCreate request: %v", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}

			if request.Config == nil || request.HostConfig == nil {
				t.Error("ContainerCreate request must include config and host config")
				http.Error(w, "missing config", http.StatusBadRequest)
				return
			}

			if request.Image != image {
				t.Errorf("ContainerCreate image = %q, want %q", request.Image, image)
			}
			if want := append([]string{"sampctl", "run"}, wantArgs...); !reflect.DeepEqual(request.Entrypoint, want) {
				t.Errorf("ContainerCreate entrypoint = %v, want %v", request.Entrypoint, want)
			}
			if !request.Tty || !request.AttachStdout || !request.AttachStderr || !request.AttachStdin {
				t.Errorf("ContainerCreate stream configuration = %+v, want tty and all streams attached", request.Config)
			}
			if !reflect.DeepEqual(request.Env, []string{"XDG_CONFIG_HOME=/root/.config"}) {
				t.Errorf("ContainerCreate environment = %v, want fixed Linux config home", request.Env)
			}
			if !strings.HasPrefix(r.URL.Query().Get("name"), "sampctl-") {
				t.Errorf("ContainerCreate name = %q, want sampctl- prefix", r.URL.Query().Get("name"))
			}

			wantMounts := []mount.Mount{
				{Type: mount.TypeBind, Source: projectDir, Target: "/samp"},
				{Type: mount.TypeBind, Source: cacheDir, Target: "/root/.config/sampctl"},
			}
			if !reflect.DeepEqual(request.HostConfig.Mounts, wantMounts) {
				t.Errorf("ContainerCreate mounts = %+v, want %+v", request.HostConfig.Mounts, wantMounts)
			}

			bindings := request.HostConfig.PortBindings[network.MustParsePort("8192/udp")]
			if len(bindings) != 1 || bindings[0].HostIP.String() != "0.0.0.0" || bindings[0].HostPort != "8192" {
				t.Errorf("ContainerCreate port bindings = %+v, want 0.0.0.0:8192 for 8192/udp", request.HostConfig.PortBindings)
			}

			createCalls++
			if createCalls == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintln(w, `{"message":"No such image"}`)
				return
			}

			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintln(w, `{"Id":"container-id"}`)
		case "POST /v1.44/images/create":
			if got := r.URL.Query().Get("fromImage"); got != "ghcr.io/southclaws/sampctl" {
				t.Errorf("ImagePull repository = %q, want ghcr.io/southclaws/sampctl", got)
			}
			if got := r.URL.Query().Get("tag"); got != "test-version" {
				t.Errorf("ImagePull tag = %q, want test-version", got)
			}
			_, _ = fmt.Fprintln(w, `{"status":"download complete"}`)
		case "POST /v1.44/containers/container-id/start":
			w.WriteHeader(http.StatusNoContent)
		case "GET /v1.44/containers/container-id/logs":
			for _, key := range []string{"stdout", "stderr", "follow"} {
				if got := r.URL.Query().Get(key); got != "1" {
					t.Errorf("ContainerLogs %s = %q, want 1", key, got)
				}
			}
			_, _ = fmt.Fprintln(w, "container output")
		case "POST /v1.44/containers/container-id/wait":
			if got := r.URL.Query().Get("condition"); got != "not-running" {
				t.Errorf("ContainerWait condition = %q, want not-running", got)
			}
			_, _ = fmt.Fprintln(w, `{"StatusCode":0}`)
		case "DELETE /v1.44/containers/container-id":
			if got := r.URL.Query().Get("force"); got != "1" {
				t.Errorf("ContainerRemove force = %q, want 1", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Docker API request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer daemon.Close()

	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(daemon.URL, "http://"))
	t.Setenv("DOCKER_API_VERSION", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	t.Setenv("DOCKER_TLS_VERIFY", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output bytes.Buffer
	options.CacheDir = cacheDir
	options.Output = &output
	err := RunContainer(ctx, run.Runtime{
		WorkingDir: workingDir,
		AppVersion: "test-version",
		Port:       &port,
		Container:  &run.ContainerConfig{MountCache: true},
	}, options)
	if err != nil {
		t.Fatalf("RunContainer(Docker 25) = %v, want nil", err)
	}
	if got := output.String(); got != "container output\n" {
		t.Errorf("RunContainer output = %q, want container output followed by newline", got)
	}

	mu.Lock()
	defer mu.Unlock()

	wantRequests := []string{
		"HEAD /_ping",
		"POST /v1.44/containers/create",
		"POST /v1.44/images/create",
		"POST /v1.44/containers/create",
		"POST /v1.44/containers/container-id/start",
		"GET /v1.44/containers/container-id/logs",
		"POST /v1.44/containers/container-id/wait",
		"DELETE /v1.44/containers/container-id",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Errorf("RunContainer requests = %v, want %v", requests, wantRequests)
	}
}

func TestStopContainerReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cli := &fakeContainerStopper{waitDelay: time.Second}
	err := stopContainer(ctx, cli, "container-id")
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, cli.killCalled)
}
