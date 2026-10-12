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

package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"

	"github.com/projectcontour/contour/internal/metrics"
)

type mockVersionChecker struct {
	err error
}

func (m *mockVersionChecker) ServerVersion() (*version.Info, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &version.Info{}, nil
}

func TestHandlerSuccess(t *testing.T) {
	m := metrics.NewMetrics(prometheus.NewRegistry())
	h := Handler(&mockVersionChecker{}, m)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerFailure(t *testing.T) {
	m := metrics.NewMetrics(prometheus.NewRegistry())
	h := Handler(&mockVersionChecker{err: errors.New("connection refused")}, m)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
