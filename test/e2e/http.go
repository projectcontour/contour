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

// +build e2e

package e2e

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// HTTP provides helpers for making HTTP/HTTPS requests.
type HTTP struct {
	// HTTPURLBase holds the IP address and port for making
	// (insecure) HTTP requests, formatted as "http://<ip>:<port>".
	HTTPURLBase string

	// HTTPSURLBase holds the IP address and port for making
	// HTTPS requests, formatted as "https://<ip>:<port>".
	HTTPSURLBase string

	// RetryInterval is how often to retry polling operations.
	RetryInterval time.Duration

	// RetryTimeout is how long to continue trying polling
	// operations before giving up.
	RetryTimeout time.Duration

	t *testing.T
}

type HTTPRequestOpts struct {
	Path        string
	Host        string
	RequestOpts []func(*http.Request)
	Condition   func(*http.Response) bool
}

func OptSetHeaders(headers map[string]string) func(*http.Request) {
	return func(r *http.Request) {
		for k, v := range headers {
			r.Header.Set(k, v)
		}
	}
}

// RequestUntil repeatedly makes HTTP requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) RequestUntil(opts *HTTPRequestOpts) (*http.Response, bool) {
	req, err := http.NewRequest("GET", h.HTTPURLBase+opts.Path, nil)
	require.NoError(h.t, err, "error creating HTTP request")

	req.Host = opts.Host
	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	makeRequest := func() (*http.Response, error) {
		return http.DefaultClient.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

type HTTPSRequestOpts struct {
	Path          string
	Host          string
	RequestOpts   []func(*http.Request)
	TLSConfigOpts []func(*tls.Config)
	Condition     func(*http.Response) bool
}

func OptSetSNI(name string) func(*tls.Config) {
	return func(c *tls.Config) {
		c.ServerName = name
	}
}

// SecureRequestUntil repeatedly makes HTTPS requests with the provided
// parameters until "condition" returns true or the timeout is reached.
// It always returns the last HTTP response received.
func (h *HTTP) SecureRequestUntil(opts *HTTPSRequestOpts) (*http.Response, bool) {
	req, err := http.NewRequest("GET", h.HTTPSURLBase+opts.Path, nil)
	require.NoError(h.t, err, "error creating HTTP request")

	req.Host = opts.Host
	for _, opt := range opts.RequestOpts {
		opt(req)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		ServerName:         opts.Host,
		InsecureSkipVerify: true,
	}
	for _, opt := range opts.TLSConfigOpts {
		opt(transport.TLSClientConfig)
	}

	client := &http.Client{
		Transport: transport,
	}

	makeRequest := func() (*http.Response, error) {
		return client.Do(req)
	}

	return h.requestUntil(makeRequest, opts.Condition)
}

func (h *HTTP) requestUntil(makeRequest func() (*http.Response, error), condition func(*http.Response) bool) (*http.Response, bool) {
	// make an immediate request and return if it succeeds
	if res, err := makeRequest(); err == nil && condition(res) {
		return res, true
	}

	// otherwise, enter a retry loop
	ticker := time.NewTicker(h.RetryInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(h.RetryTimeout)
	defer timeout.Stop()

	var res *http.Response
	var err error
	for {
		select {
		case <-ticker.C:
			res, err = makeRequest()
			if err == nil && condition(res) {
				return res, true
			}
		case <-timeout.C:
			// return the last response for logging/debugging purposes
			return res, false
		}
	}
}

// HasStatusCode returns a function that returns true
// if the response has the specified status code, or
// false otherwise.
func HasStatusCode(code int) func(*http.Response) bool {
	return func(res *http.Response) bool {
		return res != nil && res.StatusCode == code
	}
}
