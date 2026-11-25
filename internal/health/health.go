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

// Package health provides a health check service.
package health

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"

	"github.com/projectcontour/contour/internal/metrics"
)

// versionChecker is the subset of kubernetes.Clientset used by the health handler.
type versionChecker interface {
	ServerVersion() (*version.Info, error)
}

var _ versionChecker = (*kubernetes.Clientset)(nil)

// Handler returns a http Handler for a health endpoint.
func Handler(client versionChecker, m *metrics.Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := client.ServerVersion()
		if err != nil {
			m.IncHealthzRequest(false)
			msg := fmt.Sprintf("Failed Kubernetes Check: %v", err)
			http.Error(w, msg, http.StatusServiceUnavailable)
			return
		}
		m.IncHealthzRequest(true)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
}
