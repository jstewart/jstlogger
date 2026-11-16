package callbook

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// maxReplyBytes caps the size of a callbook reply.
const maxReplyBytes = 1 << 20

// getXML sends a GET request for endpoint with the query params and decodes
// the XML reply into out. A failure to reach service, a status other than
// 200 OK, or an unparseable reply is a *NetworkError. No error it returns
// holds the request URL, which may carry a password.
func getXML(ctx context.Context, client *http.Client, endpoint, service string, params url.Values, out any) (err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("callbook: %s: parse URL: %w", service, err)
	}
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("callbook: %s: build request: %w", service, redact(err))
	}
	resp, err := client.Do(req)
	if err != nil {
		return &NetworkError{Service: service, Err: redact(err)}
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("callbook: %s: close body: %w", service, cerr))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return &NetworkError{Service: service, Err: fmt.Errorf("unexpected status %s", resp.Status)}
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxReplyBytes)).Decode(out); err != nil {
		return &NetworkError{Service: service, Err: fmt.Errorf("parse reply: %w", err)}
	}
	return nil
}

// redact strips the request URL from a *url.Error, keeping its operation
// and cause.
func redact(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return fmt.Errorf("%s request: %w", ue.Op, ue.Err)
	}
	return err
}
