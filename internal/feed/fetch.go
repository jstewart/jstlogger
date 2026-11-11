package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrTooLarge reports a downloaded body longer than the caller's limit.
var ErrTooLarge = errors.New("feed: body exceeds the size limit")

// ErrUnexpectedNotModified reports a 304 Not Modified response to a request
// for a feed that has no stored copy to be unchanged from.
var ErrUnexpectedNotModified = errors.New("feed: server reported not modified, but there is no stored copy")

// Response describes a completed fetch.
type Response struct {
	// NotModified is true when the server reported the feed unchanged since
	// the stored copy; nothing was parsed.
	NotModified bool
	// ETag is the new body's ETag response header; empty when NotModified.
	ETag string
	// LastModified is the new body's Last-Modified response header; empty
	// when NotModified.
	LastModified string
}

// Fetch downloads the feed at url with client (nil means
// http.DefaultClient). prev describes the stored copy, or is the zero Meta
// when there is none; its ETag and LastModified are sent as If-None-Match
// and If-Modified-Since so an unchanged feed is not downloaded again.
//
// On 200 OK, Fetch passes the body to parse, failing with ErrTooLarge if it
// is longer than maxBytes, which must be positive, and returns the
// response's validators. A parse error is returned unchanged. On 304 Not
// Modified it returns a Response with NotModified set, or
// ErrUnexpectedNotModified if prev has never been synced. Any other status
// is an error.
func Fetch(ctx context.Context, client *http.Client, url string, prev Meta, maxBytes int64,
	parse func(io.Reader) error) (Response, error) {
	if maxBytes <= 0 {
		return Response{}, fmt.Errorf("feed: size limit %d is not positive", maxBytes)
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{}, fmt.Errorf("feed: build request: %w", err)
	}
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	if prev.LastModified != "" {
		req.Header.Set("If-Modified-Since", prev.LastModified)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("feed: download: %w", err)
	}
	return handle(resp, prev, maxBytes, parse)
}

// handle interprets resp and closes its body.
func handle(resp *http.Response, prev Meta, maxBytes int64, parse func(io.Reader) error) (res Response, err error) {
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("feed: close body: %w", cerr))
		}
	}()
	switch resp.StatusCode {
	case http.StatusNotModified:
		if prev.SyncedAt.IsZero() {
			return Response{}, ErrUnexpectedNotModified
		}
		return Response{NotModified: true}, nil
	case http.StatusOK:
		if err := parse(&cappedReader{r: resp.Body, n: maxBytes}); err != nil {
			return Response{}, err
		}
		return Response{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
		}, nil
	default:
		return Response{}, fmt.Errorf("feed: unexpected status %s", resp.Status)
	}
}

// cappedReader reads up to n bytes from r and fails with ErrTooLarge if r
// holds more, rather than ending early, so a body cut off at the limit is
// never mistaken for a whole one.
type cappedReader struct {
	r io.Reader
	n int64
}

// Read reads from the underlying reader within the limit.
func (c *cappedReader) Read(p []byte) (int, error) {
	if c.n <= 0 {
		// At the limit: the body is whole only if nothing follows.
		var one [1]byte
		if n, err := c.r.Read(one[:]); n > 0 {
			return 0, ErrTooLarge
		} else if err != nil {
			return 0, err
		}
		return 0, nil
	}
	p = p[:min(int64(len(p)), c.n)]
	n, err := c.r.Read(p)
	c.n -= int64(n)
	return n, err
}
