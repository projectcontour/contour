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

	"github.com/heptio/contour/internal/dag"
	"github.com/sirupsen/logrus"
)

const (
	holdoffDelay    = 100 * time.Millisecond
	holdoffMaxDelay = 500 * time.Millisecond
)

// A HoldoffNotifier delays calls to OnChange in the hope of
// coalescing rapid calls into a single update.
type HoldoffNotifier struct {
	// Notifier to be called after delay.
	Notifier

	logrus.FieldLogger

	mu      sync.Mutex
	timer   *time.Timer
	last    time.Time
	pending counter
}

func (hn *HoldoffNotifier) OnChange(kc *dag.KubernetesCache) {
	hn.pending.inc()
	hn.mu.Lock()
	defer hn.mu.Unlock()
	if hn.timer != nil {
		hn.timer.Stop()
	}
	since := time.Since(hn.last)
	if since > holdoffMaxDelay {
		// update immediately
		hn.WithField("last_update", since).WithField("pending", hn.pending.reset()).Info("forcing update")
		hn.Notifier.OnChange(kc)
		hn.last = time.Now()
		return
	}

	hn.timer = time.AfterFunc(holdoffDelay, func() {
		hn.mu.Lock()
		defer hn.mu.Unlock()
		hn.WithField("last_update", time.Since(hn.last)).WithField("pending", hn.pending.reset()).Info("performing delayed update")
		hn.Notifier.OnChange(kc)
		hn.last = time.Now()
	})
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) inc() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
func (c *counter) reset() uint64 {
	return atomic.SwapUint64((*uint64)(c), 0)
}
