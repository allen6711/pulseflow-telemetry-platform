package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// SelfCheck probes this process's own health endpoint over loopback and reports
// whether it answered with 200.
//
// It exists because the distroless runtime images carry no curl or wget, so the
// container healthcheck invokes the binary itself with -healthcheck.
func SelfCheck(port int, path string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building probe request for %s: %w", url, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probing %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return nil
}
