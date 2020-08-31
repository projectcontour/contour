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
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

// StatusUpdate contains an all the information needed to change an object's status to perform a specific update.
// Send down a channel to the goroutine that actually writes the changes back.
type StatusUpdate struct {
	Object   metav1.Object
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
				suh.Log.WithField("name", upd.Object.GetName()).
					WithField("namespace", upd.Object.GetNamespace()).
					Debug("not leader, not applying update")
				continue
			}

			suh.Log.WithField("name", upd.Object.GetName()).
				WithField("namespace", upd.Object.GetNamespace()).
				Debug("received a status update")

			attempts := 0
			if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				original := upd.Object
				defer func() {
					attempts++
				}()

				// If the first attempt fails due to a conflict, then refetch the resource and
				// attempt to apply our status update over that base resource.
				if attempts > 0 {
					var err error
					original, err = suh.Clients.DynamicClient().Resource(upd.Resource).Namespace(upd.Object.GetNamespace()).Get(context.TODO(), original.GetName(), metav1.GetOptions{})
					if err != nil {
						return err
					}
				}

				newObj := upd.Mutator.Mutate(original)
				if IsStatusEqual(original, newObj) {
					suh.Log.WithField("name", original.GetName()).
						WithField("namespace", original.GetNamespace()).
						Debug("Update was a no-op")
					return nil
				}

				existingBytes, err := json.Marshal(original)
				if err != nil {
					return err
				}
				updatedBytes, err := json.Marshal(newObj)
				if err != nil {
					return err
				}
				patchBytes, err := jsonpatch.CreateMergePatch(existingBytes, updatedBytes)
				if err != nil {
					return err
				}

				_, err = suh.Clients.DynamicClient().Resource(upd.Resource).Namespace(original.GetNamespace()).Patch(context.TODO(), original.GetName(), types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status")
				return err
			}); err != nil {
				suh.Log.WithError(err).
					WithField("name", upd.Object.GetName()).
					WithField("namespace", upd.Object.GetNamespace()).
					Error("unable to update status")
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
	Update(obj metav1.Object, gvr schema.GroupVersionResource, mutator StatusMutator)
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
func (suc *StatusUpdateCacher) Update(o metav1.Object, gvr schema.GroupVersionResource, mutator StatusMutator) {

	if suc.objectCache == nil {
		suc.objectCache = make(map[string]interface{})
	}

	objKey := fmt.Sprintf("%s/%s/%s", o.GetNamespace(), o.GetName(), gvr)
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
func (suw *StatusUpdateWriter) Update(obj metav1.Object, gvr schema.GroupVersionResource, mutator StatusMutator) {

	update := StatusUpdate{
		Object:   obj,
		Resource: gvr,
		Mutator:  mutator,
	}

	suw.UpdateChannel <- update
}
