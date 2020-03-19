// Copyright © 2019 VMware
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

package main

import (
	"sync"

	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

// ingressStatusWriter manages the lifetime of StatusLoadBalancerUpdaters.
//
// The theory of operation of the ingressStatusWriter is as follows:
// 1. On startup the ingressStatusWriter waits to be elected leader.
// 2. Once elected leader, the ingressStatusWriter waits to receive a
//    v1.LoadBalancerStatus value.
// 3. Once a v1.LoadBalancerStatus value has been received, any existing informer
//    is stopped and a new informer started in its place.
// 4. Each informer is connected to a k8s.StatusLoadBalancerUpdater which reacts to
//    OnAdd events for networking.k8s.io/ingress.v1beta1 objects. For each OnAdd
//    the object is patched with the v1.LoadBalancerStatus value obtained on creation.
//    OnUpdate and OnDelete events are ignored.If a new v1.LoadBalancerStatus value
//    is been received, operation restarts at step 3.
// 5. If the worker is stopped, any existing informer is stopped before the worker stops.
type ingressStatusWriter struct {
	log      logrus.FieldLogger
	clients  *k8s.Clients
	isLeader chan struct{}
	lbStatus chan v1.LoadBalancerStatus
}

func (isw *ingressStatusWriter) Start(stop <-chan struct{}) error {

	// await leadership election
	isw.log.Info("awaiting leadership election")
	select {
	case <-stop:
		// asked to stop before elected leader
		return nil
	case <-isw.isLeader:
		isw.log.Info("elected leader")
	}

	var shutdown chan struct{}
	var stopping sync.WaitGroup
	for {
		select {
		case <-stop:
			// stop existing informer and shut down
			if shutdown != nil {
				close(shutdown)
			}
			stopping.Wait()
			return nil
		case lbs := <-isw.lbStatus:
			// stop existing informer
			if shutdown != nil {
				close(shutdown)
			}
			stopping.Wait()

			// create informer for the new LoadBalancerStatus
			factory := isw.clients.NewInformerFactory()
			inf := factory.Networking().V1beta1().Ingresses().Informer()
			log := isw.log.WithField("context", "IngressStatusLoadBalancerUpdater")
			inf.AddEventHandler(&k8s.StatusLoadBalancerUpdater{
				Client: isw.clients.ClientSet(),
				Logger: log,
				Status: lbs,
			})

			shutdown = make(chan struct{})
			stopping.Add(1)
			fn := startInformer(factory, log)
			go func() {
				defer stopping.Done()
				fn(shutdown)
			}()
		}
	}
}
