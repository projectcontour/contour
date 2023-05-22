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
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

type AppPoller struct {
	cancel             context.CancelFunc
	wg                 *sync.WaitGroup
	totalRequests      uint
	successfulRequests uint
}

func StartAppPoller(address string, hostName string, expectedStatus int, errorWriter io.Writer) (*AppPoller, error) {
	ctx, cancel := context.WithCancel(context.Background())

	poller := &AppPoller{
		wg:     new(sync.WaitGroup),
		cancel: cancel,
	}

	// Disable keep alives so connections don't stay
	// open to terminating Envoy pods, which would cause
	// the shutdown-manager to block waiting for the
	// connections to drain. This lets the upgrade test
	// be more efficient.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true

	client := &http.Client{
		Transport: transport,
		Timeout:   100 * time.Millisecond,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Host = hostName

	poller.wg.Add(1)
	go func() {
		defer poller.wg.Done()
		// Ignore error here since we know we are just polling until
		// told to stop.
		_ = wait.PollUntilContextCancel(ctx, 200*time.Millisecond, true, func(ctx context.Context) (bool, error) {
			// Stop polling if we are being shut down so we don't
			// get extra failures.
			select {
			case <-ctx.Done():
				return true, nil
			default:
			}

			poller.totalRequests++
			res, err := client.Do(req)
			if err != nil {
				fmt.Fprintln(errorWriter, "error making request:", err)
				return false, nil
			}
			if res.StatusCode == expectedStatus {
				poller.successfulRequests++
			} else {
				fmt.Fprintln(errorWriter, "unexpected status code:", res.StatusCode, "response flags:", res.Header["X-Envoy-Response-Flags"])
			}
			return false, nil
		})
	}()

	return poller, nil
}

func (p *AppPoller) Stop() {
	p.cancel()
	p.wg.Wait()
}

func (p *AppPoller) Results() (uint, uint) {
	return p.totalRequests, p.successfulRequests
}
