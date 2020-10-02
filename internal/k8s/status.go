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

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	Log             logrus.FieldLogger
	Clients         *Clients
	UpdateChannel   chan StatusUpdate
	LeaderElected   chan struct{}
	IsLeader        bool
	Converter       *UnstructuredConverter
	InformerFactory InformerFactory
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

			// Fetch the lister cache for the informer associated with this resource.
			lister := suh.InformerFactory.ForResource(upd.Resource).Lister()
			uObj, err := lister.ByNamespace(upd.NamespacedName.Namespace).Get(upd.NamespacedName.Name)
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					WithField("resource", upd.Resource).
					Error("unable to retrieve object for updating")
				continue
			}

			obj, err := suh.Converter.FromUnstructured(uObj)
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					Error("unable to convert from unstructured")
				continue
			}

			newObj := upd.Mutator.Mutate(obj)

			if IsStatusEqual(obj, newObj) {
				suh.Log.WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					Debug("Update was a no-op")
				continue
			}

			usNewObj, err := suh.Converter.ToUnstructured(newObj)
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					Error("unable to convert update to unstructured")
				continue
			}

			_, err = suh.Clients.DynamicClient().Resource(upd.Resource).Namespace(upd.NamespacedName.Namespace).UpdateStatus(context.TODO(), usNewObj, metav1.UpdateOptions{})
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.NamespacedName.Name).
					WithField("namespace", upd.NamespacedName.Namespace).
					Error("unable to update status")
				continue
			}
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
