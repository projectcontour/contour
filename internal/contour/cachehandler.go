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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"time"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// CacheHandler manages the state of xDS caches.
type CacheHandler struct {
	ListenerVisitorConfig
	ListenerCache
	RouteCache
	ClusterCache
	SecretCache

	*metrics.Metrics

	logrus.FieldLogger
}

func (ch *CacheHandler) OnChange(dag *dag.DAG) {
	timer := prometheus.NewTimer(ch.CacheHandlerOnUpdateSummary)
	defer timer.ObserveDuration()

	ch.updateSecrets(dag)
	ch.updateListeners(dag)
	ch.updateRoutes(dag)
	ch.updateClusters(dag)

	ch.SetDAGLastRebuilt(time.Now())
}

func (ch *CacheHandler) updateSecrets(root dag.Visitable) {
	secrets := visitSecrets(root)
	ch.SecretCache.Update(secrets)
}

func (ch *CacheHandler) updateListeners(root dag.Visitable) {
	listeners := visitListeners(root, &ch.ListenerVisitorConfig)
	ch.ListenerCache.Update(listeners)
}

func (ch *CacheHandler) updateRoutes(root dag.Visitable) {
	routes := visitRoutes(root)
	ch.RouteCache.Update(routes)
}

func (ch *CacheHandler) updateClusters(root dag.Visitable) {
	clusters := visitClusters(root)
	ch.ClusterCache.Update(clusters)
}
