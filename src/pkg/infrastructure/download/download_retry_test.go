package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloadDoesNotRetryOn404(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	oldMax := fromNetMaxAttempts
	oldSleep := fromNetSleep
	oldClientFactory := fromNetClientFactory
	oldBackoff := fromNetBackoff
	defer func() {
		fromNetMaxAttempts = oldMax
		fromNetSleep = oldSleep
		fromNetClientFactory = oldClientFactory
		fromNetBackoff = oldBackoff
	}()

	fromNetMaxAttempts = 5
	fromNetSleep = func(time.Duration) {}
	fromNetBackoff = func(int) time.Duration { return 0 }
	fromNetClientFactory = func() *http.Client { return srv.Client() }

	tmpDir, err := os.MkdirTemp("", "sampctl-fromnet-404-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	_, err = FromNet(context.Background(), srv.URL, filepath.Join(tmpDir, "out.bin"))
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&hits), "expected exactly 1 request for non-retryable status")
}

func TestDownloadRetriesOn500ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	oldMax := fromNetMaxAttempts
	oldSleep := fromNetSleep
	oldClientFactory := fromNetClientFactory
	oldBackoff := fromNetBackoff
	defer func() {
		fromNetMaxAttempts = oldMax
		fromNetSleep = oldSleep
		fromNetClientFactory = oldClientFactory
		fromNetBackoff = oldBackoff
	}()

	fromNetMaxAttempts = 5
	fromNetSleep = func(time.Duration) {}
	fromNetBackoff = func(int) time.Duration { return 0 }
	fromNetClientFactory = func() *http.Client { return srv.Client() }

	tmpDir, err := os.MkdirTemp("", "sampctl-fromnet-500-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "out.bin")
	gotPath, err := FromNet(context.Background(), srv.URL, outPath)
	require.NoError(t, err)
	require.Equal(t, outPath, gotPath)
	require.Equal(t, int32(3), atomic.LoadInt32(&hits), "expected retries for 5xx then success")

	data, readErr := os.ReadFile(outPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("ok"), data)
}
