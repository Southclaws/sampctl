package download

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
)

func isTransientDownloadError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	if dnsErr != nil && dnsErr.IsTemporary {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT)
}

func isRetryableDownloadStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

type downloadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader downloadContextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}

	n, err := reader.reader.Read(data)
	if ctxErr := reader.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}

	return n, err
}
