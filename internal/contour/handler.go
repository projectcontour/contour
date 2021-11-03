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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventHandler implements cache.ResourceEventHandler, filters k8s events towards
// a dag.Builder and calls through to the Observer to notify it that a new DAG
// is available.
type EventHandler struct {
	Builder  dag.Builder
	Observer dag.Observer

	HoldoffDelay, HoldoffMaxDelay time.Duration

	StatusUpdater k8s.StatusUpdater

	logrus.FieldLogger

	// IsLeader will become ready to read when this EventHandler becomes
	// the leader. If IsLeader is not readable, or nil, status events will
	// be suppressed.
	IsLeader chan struct{}

	update chan interface{}

	// Sequence is a channel that receives a incrementing sequence number
	// for each update processed. The updates may be processed immediately, or
	// delayed by a holdoff timer. In each case a non blocking send to Sequence
	// will be made once the resource update is received (note
	// that the DAG is not guaranteed to be called each time).
	Sequence chan int

	// seq is the sequence counter of the number of times
	// an event has been received.
	seq int
}

type opAdd struct {
	obj interface{}
}

type opUpdate struct {
	oldObj, newObj interface{}
}

type opDelete struct {
	obj interface{}
}

func (e *EventHandler) OnAdd(obj interface{}) {
	e.update <- opAdd{obj: obj}
}

func (e *EventHandler) OnUpdate(oldObj, newObj interface{}) {
	e.update <- opUpdate{oldObj: oldObj, newObj: newObj}
}

func (e *EventHandler) OnDelete(obj interface{}) {
	e.update <- opDelete{obj: obj}
}

// UpdateNow enqueues a DAG update subject to the holdoff timer.
func (e *EventHandler) UpdateNow() {
	e.update <- true
}

// Start initializes the EventHandler and returns a function suitable
// for registration with a workgroup.Group.
func (e *EventHandler) Start() func(<-chan struct{}) error {
	e.update = make(chan interface{})
	return e.run
}

// run is the main event handling loop.
func (e *EventHandler) run(stop <-chan struct{}) error {
	e.Info("started event handler")
	defer e.Info("stopped event handler")

	var (
		// outstanding counts the number of events received but not
		// yet included in a DAG rebuild.
		outstanding int

		// timer holds the timer which will expire after e.HoldoffDelay
		timer *time.Timer

		// pending is a reference to the current timer's channel.
		pending <-chan time.Time

		// lastDAGRebuild holds the last time rebuildDAG was called.
		// lastDAGRebuild is seeded to the current time on entry to
		// run to allow the holdoff timer to batch the updates from
		// the API informers.
		lastDAGRebuild = time.Now()
	)

	reset := func() (v int) {
		v, outstanding = outstanding, 0
		return
	}

	for {
		// In the main loop one of four things can happen.
		// 1. We're waiting for an event on op, stop, or pending, noting that
		//    pending may be nil if there are no pending events.
		// 2. We're processing an event.
		// 3. The holdoff timer from a previous event has fired and we're
		//    building a new DAG and sending to the Observer.
		// 4. We're stopping.
		//
		// Only one of these things can happen at a time.
		select {
		case op := <-e.update:
			if e.onUpdate(op) {
				outstanding++
				// If there is already a timer running, stop it.
				if timer != nil {
					timer.Stop()
				}

				delay := e.HoldoffDelay
				if time.Since(lastDAGRebuild) > e.HoldoffMaxDelay {
					// the maximum holdoff delay has been exceeded so schedule the update
					// immediately by delaying for 0ns.
					delay = 0
				}
				timer = time.NewTimer(delay)
				pending = timer.C
			} else {
				// notify any watchers that we received the event but chose
				// not to process it.
				e.incSequence()
			}
		case <-pending:
			e.WithField("last_update", time.Since(lastDAGRebuild)).WithField("outstanding", reset()).Info("performing delayed update")
			e.rebuildDAG()
			e.incSequence()
			lastDAGRebuild = time.Now()
		case <-stop:
			// shutdown
			return nil
		}
	}
}

// onUpdate processes the event received. onUpdate returns
// true if the event changed the cache in a way that requires
// notifying the Observer.
func (e *EventHandler) onUpdate(op interface{}) bool {
	switch op := op.(type) {
	case opAdd:
		return e.Builder.Source.Insert(op.obj)
	case opUpdate:
		if cmp.Equal(op.oldObj, op.newObj,
			cmpopts.IgnoreFields(contour_api_v1.HTTPProxy{}, "Status"),
			cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"),
			cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ManagedFields"),
		) {
			e.WithField("op", "update").Debugf("%T skipping update, only status has changed", op.newObj)
			return false
		}
		remove := e.Builder.Source.Remove(op.oldObj)
		insert := e.Builder.Source.Insert(op.newObj)
		return remove || insert
	case opDelete:
		return e.Builder.Source.Remove(op.obj)
	case bool:
		return op
	default:
		return false
	}
}

// incSequence bumps the sequence counter and sends it to e.Sequence.
func (e *EventHandler) incSequence() {
	e.seq++
	select {
	case e.Sequence <- e.seq:
		// This is a non blocking send so if this field is nil, or the
		// receiver is not ready this send does not block incSequence's caller.
	default:
	}
}

// rebuildDAG builds a new DAG and sends it to the Observer,
// the updates the status on objects, and updates the metrics.
func (e *EventHandler) rebuildDAG() {
	latestDAG := e.Builder.Build()
	e.Observer.OnChange(latestDAG)

	for _, upd := range latestDAG.StatusCache.GetStatusUpdates() {
		e.StatusUpdater.Send(upd)
	}
}
