// Copyright © 2018 Heptio
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceEventHandler implements cache.ResourceEventHandler, filters
// k8s watcher events towards a dag.Builder (which also implements the
// same interface) and calls through to the CacheHandler to notify it
// that the contents of the dag.Builder have changed.
type ResourceEventHandler struct {
	dag.KubernetesCache

	*CacheHandler

	HoldoffDelay, HoldoffMaxDelay time.Duration

	*metrics.Metrics

	logrus.FieldLogger

	pending counter

	mu    sync.Mutex
	timer *time.Timer
	last  time.Time
}

func (reh *ResourceEventHandler) OnAdd(obj interface{}) {
	timer := prometheus.NewTimer(reh.ResourceEventHandlerSummary.With(prometheus.Labels{"op": "OnAdd"}))
	defer timer.ObserveDuration()
	reh.WithField("op", "add").Debugf("%T", obj)

	reh.KubernetesCache.Lock()
	insert := reh.Insert(obj)
	reh.KubernetesCache.Unlock()

	if insert {
		reh.update()
	}
}

func (reh *ResourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	timer := prometheus.NewTimer(reh.ResourceEventHandlerSummary.With(prometheus.Labels{"op": "OnUpdate"}))
	defer timer.ObserveDuration()
	reh.WithField("op", "update").Debugf("%T", newObj)

	reh.KubernetesCache.Lock()
	remove := reh.Remove(oldObj)
	insert := reh.Insert(newObj)
	reh.KubernetesCache.Unlock()

	if cmp.Equal(oldObj, newObj,
		cmpopts.IgnoreFields(ingressroutev1.IngressRoute{}, "Status"),
		cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")) {
		reh.WithField("op", "update").Debugf("%T skipping update, only status has changed", newObj)
		return
	}

	if insert || remove {
		reh.update()
	}
}

func (reh *ResourceEventHandler) OnDelete(obj interface{}) {
	timer := prometheus.NewTimer(reh.ResourceEventHandlerSummary.With(prometheus.Labels{"op": "OnDelete"}))
	defer timer.ObserveDuration()
	// no need to check ingress class here
	reh.WithField("op", "delete").Debugf("%T", obj)

	reh.KubernetesCache.Lock()
	remove := reh.Remove(obj)
	reh.KubernetesCache.Unlock()

	if remove {
		reh.update()
	}
}

func (reh *ResourceEventHandler) update() {
	reh.pending.inc()
	reh.mu.Lock()
	defer reh.mu.Unlock()
	if reh.timer != nil {
		reh.timer.Stop()
	}
	since := time.Since(reh.last)
	if since > reh.HoldoffMaxDelay {
		// update immediately
		reh.WithField("last_update", since).WithField("pending", reh.pending.reset()).Info("forcing update")
		reh.notify()
		return
	}

	reh.timer = time.AfterFunc(reh.HoldoffDelay, func() {
		reh.mu.Lock()
		defer reh.mu.Unlock()
		reh.WithField("last_update", time.Since(reh.last)).WithField("pending", reh.pending.reset()).Info("performing delayed update")
		reh.notify()
	})
}

func (reh *ResourceEventHandler) notify() {
	reh.CacheHandler.OnChange(&reh.KubernetesCache)
	reh.last = time.Now()
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) inc() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
func (c *counter) reset() uint64 {
	return atomic.SwapUint64((*uint64)(c), 0)
}
