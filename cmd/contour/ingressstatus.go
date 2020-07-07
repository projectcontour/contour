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

package main

import (
	"net"
	"strings"
	"sync"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
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
	log           logrus.FieldLogger
	clients       *k8s.Clients
	isLeader      chan struct{}
	lbStatus      chan v1.LoadBalancerStatus
	statusUpdater k8s.StatusUpdater
	ingressClass  string
	Converter     k8s.Converter
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

			// Configure the StatusAddressUpdater logger
			log := isw.log.WithField("context", "StatusAddressUpdater")
			if isw.ingressClass != "" {
				log = log.WithField("target-ingress-class", isw.ingressClass)
			}

			sau := &k8s.StatusAddressUpdater{
				Logger:        log,
				LBStatus:      lbs,
				IngressClass:  isw.ingressClass,
				StatusUpdater: isw.statusUpdater,
				Converter:     isw.Converter,
			}

			// Create new informer for the new LoadBalancerStatus
			factory := isw.clients.NewInformerFactory()
			factory.ForResource(v1beta1.SchemeGroupVersion.WithResource("ingresses")).Informer().AddEventHandler(sau)
			factory.ForResource(projcontour.HTTPProxyGVR).Informer().AddEventHandler(sau)

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

	// Support ','-separated lists.
	ingresses := []v1.LoadBalancerIngress{}

	for _, item := range strings.Split(status, ",") {
		item = strings.TrimSpace(item)
		if len(item) == 0 {
			continue
		}

		// Use the parseability by net.ParseIP as a signal, since we need
		// to pass a string into the v1.LoadBalancerIngress anyway.
		if ip := net.ParseIP(item); ip != nil {
			ingresses = append(ingresses, v1.LoadBalancerIngress{
				IP: item,
			})
		} else {
			ingresses = append(ingresses, v1.LoadBalancerIngress{
				Hostname: item,
			})
		}
	}

	return v1.LoadBalancerStatus{
		Ingress: ingresses,
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
