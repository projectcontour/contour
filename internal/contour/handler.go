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
	"context"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/cache/synctrack"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/k8s"
)

type EventHandlerConfig struct {
	Logger                        logrus.FieldLogger
	Builder                       *dag.Builder
	Observer                      dag.Observer
	HoldoffDelay, HoldoffMaxDelay time.Duration
	StatusUpdater                 k8s.StatusUpdater
}

// EventHandler implements cache.ResourceEventHandler, filters k8s events towards
// a dag.Builder and calls through to the Observer to notify it that a new DAG
// is available.
type EventHandler struct {
	builder  *dag.Builder
	observer dag.Observer

	holdoffDelay, holdoffMaxDelay time.Duration

	statusUpdater k8s.StatusUpdater

	logrus.FieldLogger

	update chan any

	sequence chan int

	// seq is the sequence counter of the number of times
	// an event has been received.
	seq int

	// syncTracker is used to update/query the status of the cache sync.
	// Uses an internal counter: incremented at item start, decremented at end.
	// HasSynced returns true if its UpstreamHasSynced returns true and the counter is non-positive.
	syncTracker *synctrack.SingleFileTracker

	initialDagBuilt atomic.Bool
}

func NewEventHandler(config EventHandlerConfig, upstreamHasSynced cache.InformerSynced) *EventHandler {
	return &EventHandler{
		FieldLogger:     config.Logger,
		builder:         config.Builder,
		observer:        config.Observer,
		holdoffDelay:    config.HoldoffDelay,
		holdoffMaxDelay: config.HoldoffMaxDelay,
		statusUpdater:   config.StatusUpdater,
		update:          make(chan any),
		sequence:        make(chan int, 1),
		syncTracker:     &synctrack.SingleFileTracker{UpstreamHasSynced: upstreamHasSynced},
	}
}

type opAdd struct {
	obj             any
	isInInitialList bool
}

type opUpdate struct {
	oldObj, newObj any
}

type opDelete struct {
	obj any
}

func (e *EventHandler) OnAdd(obj any, isInInitialList bool) {
	if isInInitialList {
		e.syncTracker.Start()
	}
	e.update <- opAdd{obj: obj, isInInitialList: isInInitialList}
}

func (e *EventHandler) OnUpdate(oldObj, newObj any) {
	e.update <- opUpdate{oldObj: oldObj, newObj: newObj}
}

func (e *EventHandler) OnDelete(obj any) {
	e.update <- opDelete{obj: obj}
}

// NeedLeaderElection is included to implement manager.LeaderElectionRunnable
func (e *EventHandler) NeedLeaderElection() bool {
	return false
}

func (e *EventHandler) HasBuiltInitialDag() bool {
	return e.initialDagBuilt.Load()
}

// Implements leadership.NeedLeaderElectionNotification
func (e *EventHandler) OnElectedLeader() {
	// Trigger an update when we are elected leader to ensure resource
	// statuses are not stale.
	e.update <- true
}

func (e *EventHandler) Start(ctx context.Context) error {
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

		// initialSyncPollPeriod defines the duration to wait between polling attempts during the initial informer synchronization.
		initialSyncPollPeriod = 100 * time.Millisecond

		// initialSyncPollTicker is the ticker that will trigger the periodic polling.
		initialSyncPollTicker = time.NewTicker(initialSyncPollPeriod)

		// initialSyncPoll is the channel that will receive a signal when to poll the initial informer synchronization status.
		initialSyncPoll = initialSyncPollTicker.C
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

				delay := e.holdoffDelay
				if time.Since(lastDAGRebuild) > e.holdoffMaxDelay {
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

			// We're done processing this event
			if updateOpAdd, ok := op.(opAdd); ok {
				if updateOpAdd.isInInitialList {
					e.syncTracker.Finished()
				}
			}
		case <-pending:
			// Ensure informer caches are synced.
			// Schedule a retry for dag rebuild if cache is not synced yet.
			// Note that we can't block and wait for the cache sync as it depends on progress of this loop.
			if !e.syncTracker.HasSynced() {
				e.Info("skipping dag rebuild as cache is not synced")
				timer.Reset(e.holdoffDelay)
				break
			}

			e.WithField("last_update", time.Since(lastDAGRebuild)).WithField("outstanding", reset()).Info("performing delayed update")

			// Build a new DAG and sends it to the Observer.
			latestDAG := e.builder.Build()
			e.observer.OnChange(latestDAG)

			// Update the status on objects.
			for _, upd := range latestDAG.StatusCache.GetStatusUpdates() {
				e.statusUpdater.Send(upd)
			}

			e.incSequence()
			lastDAGRebuild = time.Now()
		case <-initialSyncPoll:
			if e.syncTracker.HasSynced() {
				// Informer caches are synced, stop the polling and allow xDS server to start.
				initialSyncPollTicker.Stop()
				e.initialDagBuilt.Store(true)
			}
		case <-ctx.Done():
			// shutdown
			return nil
		}
	}
}

// onUpdate processes the event received. onUpdate returns
// true if the event changed the cache in a way that requires
// notifying the Observer.
func (e *EventHandler) onUpdate(op any) bool {
	switch op := op.(type) {
	case opAdd:
		return e.builder.Source.Insert(op.obj)
	case opUpdate:
		oldO, oldOk := op.oldObj.(client.Object)
		newO, newOk := op.newObj.(client.Object)
		if oldOk && newOk {
			equal, err := k8s.IsObjectEqual(oldO, newO)
			// Error is returned if there was no support for comparing equality of the specific object type.
			// We can still process the object but it will be always considered as changed.
			if err != nil {
				e.WithError(err).WithField("op", "update").
					WithField("name", newO.GetName()).WithField("namespace", newO.GetNamespace()).
					WithField("gvk", reflect.TypeOf(newO)).Errorf("error comparing objects")
			}
			if equal {
				// log the object name and namespace to help with debugging.
				e.WithField("op", "update").
					WithField("name", oldO.GetName()).WithField("namespace", oldO.GetNamespace()).
					WithField("gvk", reflect.TypeOf(newO)).Debugf("skipping update, no changes to relevant fields")
				return false
			}
			remove := e.builder.Source.Remove(op.oldObj)
			insert := e.builder.Source.Insert(op.newObj)
			return remove || insert
		}
		// This should never happen.
		e.WithField("op", "update").Errorf("%T skipping update, object is not a client.Object", op.newObj)
		return false
	case opDelete:
		return e.builder.Source.Remove(op.obj)
	case bool:
		return op
	default:
		return false
	}
}

// Sequence returns a channel that receives a incrementing sequence number
// for each update processed. The updates may be processed immediately, or
// delayed by a holdoff timer. In each case a non blocking send to the
// sequence channel will be made once the resource update is received (note
// that the DAG is not guaranteed to be called each time).
func (e *EventHandler) Sequence() <-chan int {
	return e.sequence
}

// incSequence bumps the sequence counter and sends it to e.Sequence.
func (e *EventHandler) incSequence() {
	e.seq++
	select {
	case e.sequence <- e.seq:
		// This is a non blocking send so if this field is nil, or the
		// receiver is not ready this send does not block incSequence's caller.
	default:
	}
}
