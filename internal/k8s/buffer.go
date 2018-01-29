// Copyright © 2017 Heptio
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
	"github.com/heptio/contour/internal/log"
	"github.com/heptio/contour/internal/workgroup"

	"k8s.io/client-go/tools/cache"
)

type buffer struct {
	ev chan interface{}
	log.Logger
	rh cache.ResourceEventHandler
}

type addEvent struct {
	obj interface{}
}

type updateEvent struct {
	oldObj, newObj interface{}
}

type deleteEvent struct {
	obj interface{}
}

// NewBuffer returns a ResourceEventHandler which buffers and serialises ResourceEventHandler events.
func NewBuffer(g *workgroup.Group, rh cache.ResourceEventHandler, log log.Logger, size int) cache.ResourceEventHandler {
	buf := &buffer{
		ev:     make(chan interface{}, size),
		Logger: log,
		rh:     rh,
	}
	g.Add(buf.loop)
	return buf
}

func (b *buffer) loop(stop <-chan struct{}) {
	log := b.WithPrefix("buffer.loop")
	log.Infof("started")
	defer log.Infof("stopped")

	for {
		select {
		case ev := <-b.ev:
			switch ev := ev.(type) {
			case *addEvent:
				b.rh.OnAdd(ev.obj)
			case *updateEvent:
				b.rh.OnUpdate(ev.oldObj, ev.newObj)
			case *deleteEvent:
				b.rh.OnDelete(ev.obj)
			default:
				log.Errorf("unhandled event type: %T: %v", ev, ev)
			}
		case <-stop:
			return
		}
	}
}

func (b *buffer) OnAdd(obj interface{}) {
	b.send(&addEvent{obj})
}

func (b *buffer) OnUpdate(oldObj, newObj interface{}) {
	b.send(&updateEvent{oldObj, newObj})
}

func (b *buffer) OnDelete(obj interface{}) {
	b.send(&deleteEvent{obj})
}

func (b *buffer) send(ev interface{}) {
	select {
	case b.ev <- ev:
		// all good
	default:
		b.Infof("event channel is full, len: %v, cap: %v", len(b.ev), cap(b.ev))
		b.ev <- ev
	}
}
