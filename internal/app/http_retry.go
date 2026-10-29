package app

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *cancelBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.cancel)
	}
	return n, err
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}

type retryTransport struct {
	base              http.RoundTripper
	retries           int
	retryDelay        time.Duration
	timeout           time.Duration
	transientStatuses map[int]bool
}

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempts := 1
	if req.Method == http.MethodGet {
		attempts += t.retries
	}
	for attempt := 0; ; attempt++ {
		attemptReq := req.Clone(req.Context())
		cancel := func() {}
		if t.timeout > 0 {
			attemptCtx, attemptCancel := context.WithTimeout(req.Context(), t.timeout)
			attemptReq = req.Clone(attemptCtx)
			cancel = attemptCancel
		}
		resp, err := t.base.RoundTrip(attemptReq)
		if err != nil {
			cancel()
		} else if resp != nil {
			resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
		}
		transientStatus := resp != nil && t.transientStatuses[resp.StatusCode]
		if attempt+1 >= attempts || (err == nil && !transientStatus) {
			return resp, err
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if t.retryDelay <= 0 {
			continue
		}
		timer := time.NewTimer(t.retryDelay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}

func newRetryHTTPClient(timeout time.Duration, retries int, retryDelay time.Duration, transientStatuses map[int]bool) *http.Client {
	if retries <= 0 {
		if timeout <= 0 {
			return http.DefaultClient
		}
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{Transport: retryTransport{
		base: http.DefaultTransport, retries: retries, retryDelay: retryDelay,
		timeout: timeout, transientStatuses: transientStatuses,
	}}
}
