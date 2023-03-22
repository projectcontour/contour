// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package e2e

import (
	"crypto/tls"
	"io"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
)

// HTTP provides helpers for making HTTP/HTTPS requests.
type HTTP struct {
	// HTTPURLBase holds the IP address and port for making
	// (insecure) HTTP requests, formatted as "http://<ip>:<port>".
	HTTPURLBase string

	// HTTPSURLBase holds the IP address and port for making
	// HTTPS requests, formatted as "https://<ip>:<port>".
	HTTPSURLBase string

	// HTTPURLMetricsBase holds the IP address and port for making
	// (insecure) HTTP requests to the Envoy metrics listener,
	// formatted as "http://<ip>:<port>".
	HTTPURLMetricsBase string

	// HTTPURLAdminBase holds the IP address and port for making
	// (insecure) HTTP requests to the Envoy admin listener,
	// formatted as "http://<ip>:<port>".
	HTTPURLAdminBase string

	// RetryInterval is how often to retry polling operations.
	RetryInterval time.Duration

	// RetryTimeout is how long to continue trying polling
	// operations before giving up.
	RetryTimeout time.Duration

	t ginkgo.GinkgoTInterface
}

type HTTPRequestOpts struct {
	Path        string
	Host        string
	OverrideURL string
	Body        io.Reader
	RequestOpts []func(*http.Request)
	ClientOpts  []func(*http.Client)
	Condition   func(*HTTPResponse) bool
}

func (o *HTTPRequestOpts) requestURLBase(defaultURL string) string {
	if o.OverrideURL != "" {
		return o.OverrideURL
	}
	return defaultURL
}

func OptSetHeaders(headers map[string]string) func(*http.Request) {
	return func(r *http.Request) {
		for k, v := range headers {
			r.Header.Set(k, v)
		}
	}
}

func OptSetQueryParams(queryParams map[string]string) func(*http.Request) {
	return func(r *http.Request) {
		q := r.URL.Query()
		for k, v := range queryParams {
			q.Add(k, v)
		}
		r.URL.RawQuery = q.Encode()
	}
}

// httpClient returns an *http.Client with its own transport
// and keep alives disabled.
func httpClient(opts ...func(*http.Client)) *http.Client {
	// Clone the DefaultTransport and disable keep alives
	// so we don't reuse connections within this method or
	// across multiple calls to this method. This helps
	// prevent requests from inadvertently being made to
	// a draining Listener.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true

	client := &http.Client{
		Transport: transport,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// RequestUntil repeatedly makes HTTP requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) RequestUntil(opts *HTTPRequestOpts) (*HTTPResponse, bool) {
	req, err := http.NewRequest(http.MethodGet, opts.requestURLBase(h.HTTPURLBase)+opts.Path, opts.Body)
	require.NoError(h.t, err, "error creating HTTP request")

	req.Host = opts.Host
	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	client := httpClient(opts.ClientOpts...)

	makeRequest := func() (*http.Response, error) {
		return client.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

func OptDontFollowRedirects(c *http.Client) {
	// Per CheckRedirect godoc: "As a special case, if
	// CheckRedirect returns ErrUseLastResponse, then
	// the most recent response is returned with its body
	// unclosed, along with a nil error."
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

// MetricsRequestUntil repeatedly makes HTTP requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) MetricsRequestUntil(opts *HTTPRequestOpts) (*HTTPResponse, bool) {
	req, err := http.NewRequest(http.MethodGet, opts.requestURLBase(h.HTTPURLMetricsBase)+opts.Path, nil)
	require.NoError(h.t, err, "error creating HTTP request")

	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	client := httpClient(opts.ClientOpts...)

	makeRequest := func() (*http.Response, error) {
		return client.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

// AdminRequestUntil repeatedly makes HTTP requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) AdminRequestUntil(opts *HTTPRequestOpts) (*HTTPResponse, bool) {
	req, err := http.NewRequest(http.MethodGet, opts.requestURLBase(h.HTTPURLAdminBase)+opts.Path, nil)
	require.NoError(h.t, err, "error creating HTTP request")

	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	client := httpClient(opts.ClientOpts...)

	makeRequest := func() (*http.Response, error) {
		return client.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

type HTTPSRequestOpts struct {
	Path          string
	Host          string
	OverrideURL   string
	Body          io.Reader
	RequestOpts   []func(*http.Request)
	TLSConfigOpts []func(*tls.Config)
	Condition     func(*HTTPResponse) bool
}

func (o *HTTPSRequestOpts) requestURLBase(defaultURL string) string {
	if o.OverrideURL != "" {
		return o.OverrideURL
	}
	return defaultURL
}

func OptSetSNI(name string) func(*tls.Config) {
	return func(c *tls.Config) {
		c.ServerName = name
	}
}

// SecureRequestUntil repeatedly makes HTTPS requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) SecureRequestUntil(opts *HTTPSRequestOpts) (*HTTPResponse, bool) {
	req, err := http.NewRequest(http.MethodGet, opts.requestURLBase(h.HTTPSURLBase)+opts.Path, opts.Body)
	require.NoError(h.t, err, "error creating HTTP request")

	req.Host = opts.Host
	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	client := httpClient()
	transport := client.Transport.(*http.Transport)

	transport.TLSClientConfig = &tls.Config{
		ServerName: opts.Host,
		//nolint:gosec
		InsecureSkipVerify: true,
	}

	for _, opt := range opts.TLSConfigOpts {
		opt(transport.TLSClientConfig)
	}

	makeRequest := func() (*http.Response, error) {
		return client.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

// SecureRequest makes a single HTTPS request with the provided parameters
// and returns the HTTP response or an error. Note that opts.Condition is
// ignored by this method.
//
// In general, E2E's should use SecureRequestUntil instead of this method since
// SecureRequestUntil will retry requests to account for eventual consistency and
// other ephemeral issues.
func (h *HTTP) SecureRequest(opts *HTTPSRequestOpts) (*HTTPResponse, error) {
	req, err := http.NewRequest(http.MethodGet, opts.requestURLBase(h.HTTPSURLBase)+opts.Path, nil)
	require.NoError(h.t, err, "error creating HTTP request")

	req.Host = opts.Host
	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		ServerName: opts.Host,
		//nolint:gosec
		InsecureSkipVerify: true,
	}
	for _, opt := range opts.TLSConfigOpts {
		opt(transport.TLSClientConfig)
	}

	client := &http.Client{
		Transport: transport,
	}

	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	bodyBytes, err := io.ReadAll(r.Body)
	require.NoError(h.t, err)

	return &HTTPResponse{
		StatusCode: r.StatusCode,
		Headers:    r.Header,
		Body:       bodyBytes,
	}, nil
}

func (h *HTTP) requestUntil(makeRequest func() (*http.Response, error), condition func(*HTTPResponse) bool) (*HTTPResponse, bool) {
	var res *HTTPResponse

	if err := wait.PollImmediate(h.RetryInterval, h.RetryTimeout, func() (bool, error) {
		r, err := makeRequest()
		if err != nil {
			// if there was an error, we want to keep
			// retrying, so just return false, not an
			// error.
			return false, nil
		}
		defer r.Body.Close()

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(h.t, err)

		res = &HTTPResponse{
			StatusCode: r.StatusCode,
			Headers:    r.Header,
			Body:       bodyBytes,
		}

		if condition != nil {
			return condition(res), nil
		}
		return false, nil
	}); err != nil {
		return res, false
	}

	return res, true
}

type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// HasStatusCode returns a function that returns true
// if the response has the specified status code, or
// false otherwise.
func HasStatusCode(code int) func(*HTTPResponse) bool {
	return func(res *HTTPResponse) bool {
		return res != nil && res.StatusCode == code
	}
}
