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

package main

import (
	"net"
	"sync"

	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

// loadBalancerStatusWriter manages the lifetime of IngressStatusUpdaters.
//
// The theory of operation of the loadBalancerStatusWriter is as follows:
// 1. On startup the loadBalancerStatusWriter waits to be elected leader.
// 2. Once elected leader, the loadBalancerStatusWriter waits to receive a
//    v1.LoadBalancerStatus value.
// 3. Once a v1.LoadBalancerStatus value has been received, any existing informer
//    is stopped and a new informer started in its place. This ensures that all existing
//    Ingress objects will have OnAdd events fired to the new event handler.
// 4. Each informer is connected to a k8s.IngressStatusUpdater which reacts to
//    OnAdd events for networking.k8s.io/ingress.v1beta1 objects. For each OnAdd
//    the object is patched with the v1.LoadBalancerStatus value obtained on creation.
//    OnUpdate and OnDelete events are ignored.If a new v1.LoadBalancerStatus value
//    is been received, operation restarts at step 3.
// 5. If the worker is stopped, any existing informer is stopped before the worker stops.
type loadBalancerStatusWriter struct {
	log      logrus.FieldLogger
	clients  *k8s.Clients
	isLeader chan struct{}
	lbStatus chan v1.LoadBalancerStatus
}

func (isw *loadBalancerStatusWriter) Start(stop <-chan struct{}) error {

	// Await leadership election.
	isw.log.Info("awaiting leadership election")
	select {
	case <-stop:
		// We were asked to stop before elected leader.
		return nil
	case <-isw.isLeader:
		isw.log.Info("elected leader")
	}

	var shutdown chan struct{}
	var ingressInformers sync.WaitGroup
	for {
		select {
		case <-stop:
			// Use the shutdown channel to stop existing informer and shut down
			if shutdown != nil {
				close(shutdown)
			}
			ingressInformers.Wait()
			return nil
		case lbs := <-isw.lbStatus:
			// Stop the existing informer.
			if shutdown != nil {
				close(shutdown)
			}
			ingressInformers.Wait()

			isw.log.WithField("loadbalancer-address", lbAddress(lbs)).Info("received a new address for status.loadBalancer")

			// Create new informer for the new LoadBalancerStatus
			factory := isw.clients.NewInformerFactory()
			inf := factory.Networking().V1beta1().Ingresses().Informer()
			log := isw.log.WithField("context", "IngressStatusUpdater")
			inf.AddEventHandler(&k8s.IngressStatusUpdater{
				Client: isw.clients.ClientSet(),
				Logger: log,
				Status: lbs,
			})

			shutdown = make(chan struct{})
			ingressInformers.Add(1)
			fn := startInformer(factory, log)
			go func() {
				defer ingressInformers.Done()
				if err := fn(shutdown); err != nil {
					return
				}
			}()
		}
	}
}

func parseStatusFlag(status string) v1.LoadBalancerStatus {

	// Use the parseability by net.ParseIP as a signal, since we need
	// to pass a string into the v1.LoadBalancerIngress anyway.
	if ip := net.ParseIP(status); ip != nil {
		return v1.LoadBalancerStatus{
			Ingress: []v1.LoadBalancerIngress{
				{
					IP: status,
				},
			},
		}
	}

	return v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: status,
			},
		},
	}
}

// lbAddress gets the string representation of the first address, for logging.
func lbAddress(lb v1.LoadBalancerStatus) string {

	if len(lb.Ingress) == 0 {
		return ""
	}

	if lb.Ingress[0].IP != "" {
		return lb.Ingress[0].IP
	}

	return lb.Ingress[0].Hostname
}
