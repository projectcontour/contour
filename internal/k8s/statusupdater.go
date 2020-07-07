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
)

// StatusUpdate contains an all the information needed to change an object's status to perform a specific update.
// Send down a channel to the goroutine that actually writes the changes back.
type StatusUpdate struct {
	FullName FullName
	Resource schema.GroupVersionResource
	Mutator  StatusMutator
}

// StatusMutator is an interface to hold mutator functions for status updates.
type StatusMutator interface {
	Mutate(obj interface{}) interface{}
}

// StatusMutatorFunc is a function adaptor for StatusMutators.
type StatusMutatorFunc func(interface{}) interface{}

// Mutate adapts the StatusMutatorFunc to fit through the StatusMutator inferface.
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
				suh.Log.WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					Debug("not leader, not applying update")
				continue
			}

			suh.Log.WithField("name", upd.FullName.Name).
				WithField("namespace", upd.FullName.Namespace).
				Debug("received a status update")
			uObj, err := suh.Clients.DynamicClient().
				Resource(upd.Resource).
				Namespace(upd.FullName.Namespace).Get(context.TODO(), upd.FullName.Name, metav1.GetOptions{})
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					WithField("resource", upd.Resource).
					Error("unable to retrieve object for updating")
				continue
			}

			obj, err := suh.Converter.FromUnstructured(uObj)
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					Error("unable to convert from unstructured")
				continue
			}

			newObj := upd.Mutator.Mutate(obj)

			if IsStatusEqual(obj, newObj) {
				suh.Log.WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					Debug("Update was a no-op")
				continue
			}

			usNewObj, err := suh.Converter.ToUnstructured(newObj)
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					Error("unable to convert update to unstructured")
				continue
			}

			_, err = suh.Clients.DynamicClient().Resource(upd.Resource).Namespace(upd.FullName.Namespace).UpdateStatus(context.TODO(), usNewObj, metav1.UpdateOptions{})
			if err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.FullName.Name).
					WithField("namespace", upd.FullName.Namespace).
					Error("unable to update status")
				continue
			}

		}

	}

}

// Writer retrieves the interface that should be used to write to the StatusUpdateHandler.
func (suh *StatusUpdateHandler) Writer() StatusUpdater {

	if suh.UpdateChannel == nil {
		suh.UpdateChannel = make(chan StatusUpdate)
	}

	return &StatusUpdateWriter{
		UpdateChannel: suh.UpdateChannel,
	}
}

// StatusUpdater describes an interface to send status updates somewhere.
type StatusUpdater interface {
	Update(name, namespace string, gvr schema.GroupVersionResource, mutator StatusMutator)
}

// StatusUpdateCacher takes status updates and applies them to a cache, to be used for testing.
type StatusUpdateCacher struct {
	objectCache map[string]interface{}
}

// GetObject allows retrieval of objects from the cache.
func (suc *StatusUpdateCacher) GetObject(name, namespace string, gvr schema.GroupVersionResource) interface{} {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	obj, ok := suc.objectCache[suc.objectPrefix(name, namespace, gvr)]
	if ok {
		return obj
	}
	return nil

}

func (suc *StatusUpdateCacher) AddObject(name, namespace string, gvr schema.GroupVersionResource, obj interface{}) bool {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	prefix := suc.objectPrefix(name, namespace, gvr)
	_, ok := suc.objectCache[prefix]
	if ok {
		return false
	}

	suc.objectCache[prefix] = obj

	return true

}

func (suc *StatusUpdateCacher) objectPrefix(name, namespace string, gvr schema.GroupVersionResource) string {
	return fmt.Sprintf("%s/%s/%s", namespace, name, gvr)
}

// Update updates the cache with the requested update.
func (suc *StatusUpdateCacher) Update(name, namespace string, gvr schema.GroupVersionResource, mutator StatusMutator) {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	objKey := fmt.Sprintf("%s/%s/%s", namespace, name, gvr)
	obj, ok := suc.objectCache[objKey]
	if ok {
		suc.objectCache[objKey] = mutator.Mutate(obj)
	}
}

// StatusUpdateWriter takes status updates and sends these to the StatusUpdateHandler via a channel.
type StatusUpdateWriter struct {
	UpdateChannel chan StatusUpdate
}

// Update sends the update to the StatusUpdateHandler for delivery to the apiserver.
// The StatusUpdateHandler will retrieve the object, update it with the mutator, then put it back if it's different.
func (suw *StatusUpdateWriter) Update(name, namespace string, gvr schema.GroupVersionResource, mutator StatusMutator) {

	update := StatusUpdate{
		FullName: FullName{
			Name:      name,
			Namespace: namespace,
		},
		Resource: gvr,
		Mutator:  mutator,
	}

	suw.UpdateChannel <- update
}
