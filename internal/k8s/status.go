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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusUpdate contains an all the information needed to change an object's status to perform a specific update.
// Send down a channel to the goroutine that actually writes the changes back.
type StatusUpdate struct {
	NamespacedName types.NamespacedName
	Resource       client.Object
	Mutator        StatusMutator
}

func NewStatusUpdate(name, namespace string, resource client.Object, mutator StatusMutator) StatusUpdate {
	return StatusUpdate{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
		Resource: resource,
		Mutator:  mutator,
	}
}

// StatusMutator is an interface to hold mutator functions for status updates.
type StatusMutator interface {
	Mutate(obj client.Object) client.Object
}

// StatusMutatorFunc is a function adaptor for StatusMutators.
type StatusMutatorFunc func(client.Object) client.Object

// Mutate adapts the StatusMutatorFunc to fit through the StatusMutator interface.
func (m StatusMutatorFunc) Mutate(old client.Object) client.Object {
	if m == nil {
		return nil
	}

	return m(old)
}

// StatusUpdateHandler holds the details required to actually write an Update back to the referenced object.
type StatusUpdateHandler struct {
	log           logrus.FieldLogger
	client        client.Client
	sendUpdates   chan struct{}
	updateChannel chan StatusUpdate
}

func NewStatusUpdateHandler(log logrus.FieldLogger, client client.Client) *StatusUpdateHandler {
	return &StatusUpdateHandler{
		log:           log,
		client:        client,
		sendUpdates:   make(chan struct{}),
		updateChannel: make(chan StatusUpdate, 100),
	}
}

func (suh *StatusUpdateHandler) apply(upd StatusUpdate) {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		obj := upd.Resource

		// Get the resource.
		if err := suh.client.Get(context.Background(), upd.NamespacedName, obj); err != nil {
			return err
		}

		newObj := upd.Mutator.Mutate(obj)

		if isStatusEqual(obj, newObj) {
			suh.log.WithField("name", upd.NamespacedName.Name).
				WithField("namespace", upd.NamespacedName.Namespace).
				Debug("update was a no-op")
			return nil
		}

		return suh.client.Status().Update(context.Background(), newObj)
	}); err != nil {
		suh.log.WithError(err).
			WithField("name", upd.NamespacedName.Name).
			WithField("namespace", upd.NamespacedName.Namespace).
			Error("unable to update status")
	}
}

func (suh *StatusUpdateHandler) NeedLeaderElection() bool {
	return true
}

// Start runs the goroutine to perform status writes.
func (suh *StatusUpdateHandler) Start(ctx context.Context) error {
	suh.log.Info("started status update handler")
	defer suh.log.Info("stopped status update handler")

	// Enable StatusUpdaters to start sending updates to this handler.
	close(suh.sendUpdates)

	for {
		select {
		case <-ctx.Done():
			return nil
		case upd := <-suh.updateChannel:
			suh.log.WithField("name", upd.NamespacedName.Name).
				WithField("namespace", upd.NamespacedName.Namespace).
				Debug("received a status update")

			suh.apply(upd)
		}

	}

}

// Writer retrieves the interface that should be used to write to the StatusUpdateHandler.
func (suh *StatusUpdateHandler) Writer() StatusUpdater {
	return &StatusUpdateWriter{
		enabled:       suh.sendUpdates,
		updateChannel: suh.updateChannel,
	}
}

// StatusUpdater describes an interface to send status updates somewhere.
type StatusUpdater interface {
	Send(su StatusUpdate)
}

// StatusUpdateWriter takes status updates and sends these to the StatusUpdateHandler via a channel.
type StatusUpdateWriter struct {
	enabled       <-chan struct{}
	updateChannel chan<- StatusUpdate
}

// Send sends the given StatusUpdate off to the update channel for writing by the StatusUpdateHandler.
func (suw *StatusUpdateWriter) Send(update StatusUpdate) {
	// Non-blocking recieve to see if we should pass along update.
	select {
	case <-suw.enabled:
		suw.updateChannel <- update
	default:
	}
}
