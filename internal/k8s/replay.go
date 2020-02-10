// Copyright © 2020 VMware
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

package k8s

import (
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	"k8s.io/client-go/tools/cache"
)

type event struct {
	Offset    int64
	Timestamp string

	// Event is "add", "delete" or "update"
	Event   string
	Objects []string
}

// EventRecorder is a type that records Kubernetes events. eventName
// should be either "add", "delete" or "update", corresponding with
// the operations of the cache.ResourceEventHandler interface. objList
// is the list of objects participating in the event. Update events
// should have 2 events in objList (the old and new objects); other
// operations should only have 1 event.
type EventRecorder interface {
	Record(eventName string, objList ...interface{})
}

// RecordingHandlerFactory is an interface that generates a
// cache.ResourceHandler that wraps is parameter to return a new
// handler that records events before sending them on.
type RecordingHandlerFactory interface {
	NewHandler(handler cache.ResourceEventHandler) cache.ResourceEventHandler
}

type eventRecorder struct {
	startTime time.Time
	lock      sync.Mutex
	output    *os.File
}

var _ EventRecorder = &eventRecorder{}
var _ RecordingHandlerFactory = &eventRecorder{}

// Record writes the named event and the given list of objects parameters to an event log.
func (e *eventRecorder) Record(eventName string, objList ...interface{}) {
	e.writeEvent(e.nextEvent("add", objList...))
}

// NewHandler returns a new Kubernetes cache.ResourceEventHandler
// that records all events before passing them along to the handler h.
func (e *eventRecorder) NewHandler(h cache.ResourceEventHandler) cache.ResourceEventHandler {
	return &recordingHandler{
		recorder: e,
		handler:  h,
	}
}

func (e eventRecorder) nextEvent(eventName string, objList ...interface{}) event {
	now := time.Now()
	ev := event{
		Offset:    int64(now.Sub(e.startTime)),
		Timestamp: now.Format(time.RFC3339),
		Event:     eventName,
		Objects:   []string{},
	}

	for _, o := range objList {
		data, err := yaml.Marshal(o)
		if err != nil {
			panic(err.Error())
		}

		ev.Objects = append(ev.Objects, string(data))
	}

	return ev
}

func (e eventRecorder) writeEvent(ev event) {
	e.lock.Lock()
	defer e.lock.Unlock()

	// TODO(jpeach): Consider using a persistent encoder.
	encoder := yaml.NewEncoder(e.output)

	e.output.Write([]byte("---\n"))

	encoder.Encode(ev)
	encoder.Close()

	e.output.Sync()
}

type recordingHandler struct {
	recorder EventRecorder
	handler  cache.ResourceEventHandler
}

var _ cache.ResourceEventHandler = &recordingHandler{}

func (e recordingHandler) OnAdd(obj interface{}) {
	e.recorder.Record("add", obj)
	e.handler.OnAdd(obj)
}

func (e recordingHandler) OnUpdate(oldObj, newObj interface{}) {
	e.recorder.Record("update", oldObj, newObj)
	e.handler.OnUpdate(oldObj, newObj)
}

func (e recordingHandler) OnDelete(obj interface{}) {
	e.recorder.Record("delete", obj)
	e.handler.OnDelete(obj)
}

// NewEventRecorder returns a factory that generates event handlers
// that log events to "events.log".
func NewEventRecorder() (RecordingHandlerFactory, error) {
	// TODO(jpeach): add filename option.
	fileName := "events.log"

	// TODO(jpeach): add compression option.

	fh, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &eventRecorder{
		startTime: time.Now(),
		output:    fh,
	}, nil
}
