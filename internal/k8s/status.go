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

package k8s

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

// StatusUpdate contains an all the information needed to change an object's status to perform a specific update.
// Send down a channel to the goroutine that actually writes the changes back.
type StatusUpdate struct {
	NamespacedName types.NamespacedName
	Resource       schema.GroupVersionResource
	Mutator        StatusMutator
}

func NewStatusUpdate(name, namespace string, gvr schema.GroupVersionResource, mutator StatusMutator) StatusUpdate {
	return StatusUpdate{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
		Resource: gvr,
		Mutator:  mutator,
	}
}

// StatusMutator is an interface to hold mutator functions for status updates.
type StatusMutator interface {
	Mutate(obj interface{}) interface{}
}

// StatusMutatorFunc is a function adaptor for StatusMutators.
type StatusMutatorFunc func(interface{}) interface{}

// Mutate adapts the StatusMutatorFunc to fit through the StatusMutator interface.
func (m StatusMutatorFunc) Mutate(old interface{}) interface{} {
	if m == nil {
		return nil
	}

	return m(old)
}

// StatusUpdateHandler holds the details required to actually write an Update back to the referenced object.
type StatusUpdateHandler struct {
	Log           logrus.FieldLogger
	Clients       *Clients
	UpdateChannel chan StatusUpdate
	LeaderElected chan struct{}
	IsLeader      bool
	Converter     *UnstructuredConverter
}

func (suh *StatusUpdateHandler) apply(upd StatusUpdate) {
	gvk, err := suh.Clients.KindFor(upd.Resource)
	if err != nil {
		suh.Log.WithError(err).
			WithField("name", upd.NamespacedName.Name).
			WithField("namespace", upd.NamespacedName.Namespace).
			WithField("resource", upd.Resource).
			Error("failed to map Resource to Kind ")
		return
	}

	obj, err := suh.Converter.scheme.New(gvk)
	if err != nil {
		suh.Log.WithError(err).
			WithField("name", upd.NamespacedName.Name).
			WithField("namespace", upd.NamespacedName.Namespace).
			WithField("kind", gvk).
			Error("failed to allocate template object")
		return
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Fetch the lister cache for the informer associated with this resource.
		if err := suh.Clients.Cache().Get(context.Background(), upd.NamespacedName, obj); err != nil {
			return err
		}

		newObj := upd.Mutator.Mutate(obj)

		if IsStatusEqual(obj, newObj) {
			suh.Log.WithField("name", upd.NamespacedName.Name).
				WithField("namespace", upd.NamespacedName.Namespace).
				Debug("update was a no-op")
			return nil
		}

		usNewObj, err := suh.Converter.ToUnstructured(newObj)
		if err != nil {
			return fmt.Errorf("unable to convert object: %w", err)
		}

		_, err = suh.Clients.DynamicClient().
			Resource(upd.Resource).
			Namespace(upd.NamespacedName.Namespace).
			UpdateStatus(context.Background(), usNewObj, metav1.UpdateOptions{})
		return err
	}); err != nil {
		suh.Log.WithError(err).
			WithField("name", upd.NamespacedName.Name).
			WithField("namespace", upd.NamespacedName.Namespace).
			Error("unable to update status")
	}
}

// Start runs the goroutine to perform status writes.
// Until the Contour is elected leader, will drop updates on the floor.
func (suh *StatusUpdateHandler) Start(stop <-chan struct{}) error {
	for {
		select {
		case <-stop:
			return nil
		case <-suh.LeaderElected:
			suh.Log.Info("elected leader")
			suh.IsLeader = true
			// disable this case
			suh.LeaderElected = nil
		case upd := <-suh.UpdateChannel:
			if !suh.IsLeader {
				suh.Log.WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					Debug("not leader, not applying update")
				continue
			}

			suh.Log.WithField("name", upd.NamespacedName.Name).
				WithField("namespace", upd.NamespacedName.Namespace).
				Debug("received a status update")

			suh.apply(upd)
		}

	}

}

// Writer retrieves the interface that should be used to write to the StatusUpdateHandler.
func (suh *StatusUpdateHandler) Writer() StatusUpdater {
	if suh.UpdateChannel == nil {
		suh.UpdateChannel = make(chan StatusUpdate, 100)
	}

	return &StatusUpdateWriter{
		UpdateChannel: suh.UpdateChannel,
	}
}

// StatusUpdater describes an interface to send status updates somewhere.
type StatusUpdater interface {
	Send(su StatusUpdate)
}

// StatusUpdateWriter takes status updates and sends these to the StatusUpdateHandler via a channel.
type StatusUpdateWriter struct {
	UpdateChannel chan StatusUpdate
}

// Send sends the given StatusUpdate off to the update channel for writing by the StatusUpdateHandler.
func (suw *StatusUpdateWriter) Send(update StatusUpdate) {
	suw.UpdateChannel <- update
}
