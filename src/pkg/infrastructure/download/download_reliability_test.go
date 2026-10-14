package download

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDownloadTransportFailureRetriesOnlyTransientErrors(t *testing.T) {
	oldBackoff := fromNetBackoff
	fromNetBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { fromNetBackoff = oldBackoff })

	for _, test := range []struct {
		name         string
		err          error
		wantAttempts int
	}{
		{name: "timeout", err: &net.DNSError{Err: "timeout", IsTimeout: true}, wantAttempts: fromNetMaxAttempts},
		{name: "connection refused", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, wantAttempts: fromNetMaxAttempts},
		{name: "unknown permanent error", err: errors.New("invalid transport configuration"), wantAttempts: 1},
		{name: "missing host", err: &net.DNSError{Err: "no such host", IsNotFound: true}, wantAttempts: 1},
		{name: "canceled", err: context.Canceled, wantAttempts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var attempts int
			client := roundTripClient(func(*http.Request) (*http.Response, error) {
				attempts++
				return nil, test.err
			})

			_, err := FromNetWithClient(context.Background(), client, "https://example.invalid/file", filepath.Join(t.TempDir(), "artifact"))
			if !errors.Is(err, test.err) {
				t.Errorf("FromNetWithClient() error = %v, want cause %v", err, test.err)
			}
			if attempts != test.wantAttempts {
				t.Errorf("FromNetWithClient() attempts = %d, want %d", attempts, test.wantAttempts)
			}
		})
	}
}

func TestDownloadPermanentWriteFailureIsNotRetried(t *testing.T) {
	oldBackoff := fromNetBackoff
	fromNetBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { fromNetBackoff = oldBackoff })

	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	var attempts int
	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		attempts++
		return jsonResponse(http.StatusOK, "download"), nil
	})

	_, err := FromNetWithClient(context.Background(), client, "https://example.invalid/file", filepath.Join(parent, "artifact"))
	if err == nil {
		t.Fatal("FromNetWithClient() error = nil, want invalid filesystem path")
	}
	if attempts != 1 {
		t.Errorf("FromNetWithClient() attempts = %d, want 1", attempts)
	}

	data, err := os.ReadFile(parent)
	if err != nil || string(data) != "existing" {
		t.Errorf("ReadFile(parent) = %q, %v, want existing file preserved", data, err)
	}
}

type cancelingDownloadBody struct {
	cancel context.CancelFunc
}

func (body cancelingDownloadBody) Read(data []byte) (int, error) {
	body.cancel()
	return copy(data, "partial"), nil
}

func (cancelingDownloadBody) Close() error { return nil }

func TestDownloadCancellationDuringBodyPreservesPreviousCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusOK, "")
		response.Body = cancelingDownloadBody{cancel: cancel}
		return response, nil
	})

	_, err := FromNetWithClient(ctx, client, "https://example.invalid/file", path)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("FromNetWithClient(body canceled) = %v, want cancellation", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "existing" {
		t.Errorf("ReadFile(previous cache) = %q, %v, want existing", contents, err)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadDir() = %v, %v, want no partial cache artifacts", entries, err)
	}
}

type interruptedDownloadBody struct{}

func (interruptedDownloadBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (interruptedDownloadBody) Close() error             { return nil }

func TestDownloadInterruptedBodyPreservesCacheUntilRetrySucceeds(t *testing.T) {
	oldBackoff := fromNetBackoff
	fromNetBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { fromNetBackoff = oldBackoff })

	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	var attempts int
	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		attempts++
		response := jsonResponse(http.StatusOK, "complete")
		if attempts == 1 {
			response.Body = interruptedDownloadBody{}
		} else {
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "existing" {
				t.Errorf("ReadFile(cache during retry) = %q, %v, want existing", data, err)
			}
		}

		return response, nil
	})

	_, err := FromNetWithClient(context.Background(), client, "https://example.invalid/file", path)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil || string(data) != "complete" || attempts != 2 {
		t.Errorf("download = %q, %v after %d attempts, want complete after 2", data, err, attempts)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadDir(cache) = %v, %v, want only completed artifact", entries, err)
	}
}

func TestDownloadCanceledBeforeTransferDoesNotWriteCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var attempts int
	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		attempts++
		return jsonResponse(http.StatusOK, "complete"), nil
	})
	path := filepath.Join(t.TempDir(), "artifact")

	_, err := FromNetWithClient(ctx, client, "https://example.invalid/file", path)
	if !errors.Is(err, context.Canceled) || attempts != 0 {
		t.Errorf("FromNetWithClient(canceled) = %v after %d attempts, want canceled before transfer", err, attempts)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(cache) = %v, want no cached artifact", err)
	}
}

func TestDownloadInvalidContentTypeIsNotRetried(t *testing.T) {
	oldBackoff := fromNetBackoff
	fromNetBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { fromNetBackoff = oldBackoff })

	var attempts int
	client := roundTripClient(func(*http.Request) (*http.Response, error) {
		attempts++
		response := jsonResponse(http.StatusOK, "png")
		response.Header.Set("Content-Type", "image/png")
		return response, nil
	})

	_, err := FromNetWithClient(context.Background(), client, "https://example.invalid/file", filepath.Join(t.TempDir(), "artifact"))
	if err == nil || !strings.Contains(err.Error(), "content type") || attempts != 1 {
		t.Errorf("FromNetWithClient(image/png) = %v after %d attempts, want permanent content type error", err, attempts)
	}
}
