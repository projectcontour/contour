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
	"context"
	"net"
	"strings"

	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
)

// loadBalancerStatusWriter orchestrates LoadBalancer address status
// updates for HTTPProxy, Ingress and Gateway objects. Actually updating the
// address in the object status is performed by k8s.StatusAddressUpdater.
//
// The theory of operation of the loadBalancerStatusWriter is as follows:
//
//  1. On startup the loadBalancerStatusWriter waits to be elected leader.
//  2. Once elected leader, the loadBalancerStatusWriter waits to receive a
//     core_v1.LoadBalancerStatus value.
//  3. Once a core_v1.LoadBalancerStatus value has been received, the
//     cached address is updated so that it will be applied to objects
//     received in any subsequent informer events.
//  4. All Ingress, HTTPProxy and Gateway objects are listed from the informer
//     cache and an attempt is made to update their status with the new
//     address. This update may end up being a no-op in which case it
//     doesn't make an API server call.
//  5. If the worker is stopped, the informer continues but no further
//     status updates are made.
type loadBalancerStatusWriter struct {
	log               logrus.FieldLogger
	cache             cache.Cache
	lbStatus          chan core_v1.LoadBalancerStatus
	statusUpdater     k8s.StatusUpdater
	ingressClassNames []string
	gatewayRef        *types.NamespacedName
}

func (isw *loadBalancerStatusWriter) NeedLeaderElection() bool {
	return true
}

func (isw *loadBalancerStatusWriter) Start(ctx context.Context) error {
	u := &k8s.StatusAddressUpdater{
		Logger: func() logrus.FieldLogger {
			// Configure the StatusAddressUpdater logger.
			log := isw.log.WithField("context", "StatusAddressUpdater")
			if len(isw.ingressClassNames) > 0 {
				return log.WithField("target-ingress-classes", isw.ingressClassNames)
			}

			return log
		}(),
		Cache:             isw.cache,
		IngressClassNames: isw.ingressClassNames,
		GatewayRef:        isw.gatewayRef,
		StatusUpdater:     isw.statusUpdater,
	}

	// Create informers for the types that need load balancer
	// address status. The cache should have already started
	// informers, so new informers will auto-start.
	resources := []client.Object{
		&contour_v1.HTTPProxy{},
		&networking_v1.Ingress{},
	}

	// Only create Gateway informer if a gateway was provided,
	// otherwise the API may not exist in the cluster.
	if isw.gatewayRef != nil {
		resources = append(resources, &gatewayapi_v1.Gateway{})
	}

	for _, r := range resources {
		inf, err := isw.cache.GetInformer(context.Background(), r)
		if err != nil {
			isw.log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}

		_, err = inf.AddEventHandler(u)
		if err != nil {
			isw.log.WithError(err).WithField("resource", r).Fatal("failed to add event handler to informer")
		}
	}

	for {
		select {
		case <-ctx.Done():
			// Once started, there's no way to stop the
			// informer from here. Clear the load balancer
			// status so that subsequent informer events
			// will have no effect.
			u.Set(core_v1.LoadBalancerStatus{})
			return nil
		case lbs := <-isw.lbStatus:
			isw.log.WithField("loadbalancer-address", lbAddress(lbs)).
				Info("received a new address for status.loadBalancer")

			u.Set(lbs)

			var ingressList networking_v1.IngressList
			if err := isw.cache.List(context.Background(), &ingressList); err != nil {
				isw.log.WithError(err).WithField("kind", "Ingress").Error("failed to list objects")
			} else {
				for i := range ingressList.Items {
					u.OnAdd(&ingressList.Items[i], false)
				}
			}

			var proxyList contour_v1.HTTPProxyList
			if err := isw.cache.List(context.Background(), &proxyList); err != nil {
				isw.log.WithError(err).WithField("kind", "HTTPProxy").Error("failed to list objects")
			} else {
				for i := range proxyList.Items {
					u.OnAdd(&proxyList.Items[i], false)
				}
			}

			// Only list Gateways if a gateway was configured,
			// otherwise the API may not exist in the cluster.
			if isw.gatewayRef != nil {
				var gatewayList gatewayapi_v1.GatewayList
				if err := isw.cache.List(context.Background(), &gatewayList); err != nil {
					isw.log.WithError(err).WithField("kind", "Gateway").Error("failed to list objects")
				} else {
					for i := range gatewayList.Items {
						u.OnAdd(&gatewayList.Items[i], false)
					}
				}
			}
		}
	}
}

func parseStatusFlag(status string) core_v1.LoadBalancerStatus {
	// Support ','-separated lists.
	var ingresses []core_v1.LoadBalancerIngress

	for _, item := range strings.Split(status, ",") {
		item = strings.TrimSpace(item)
		if len(item) == 0 {
			continue
		}

		// Use the parseability by net.ParseIP as a signal, since we need
		// to pass a string into the core_v1.LoadBalancerIngress anyway.
		if ip := net.ParseIP(item); ip != nil {
			ingresses = append(ingresses, core_v1.LoadBalancerIngress{
				IP: item,
			})
		} else {
			ingresses = append(ingresses, core_v1.LoadBalancerIngress{
				Hostname: item,
			})
		}
	}

	return core_v1.LoadBalancerStatus{
		Ingress: ingresses,
	}
}

// lbAddress gets the string representation of the first address, for logging.
func lbAddress(lb core_v1.LoadBalancerStatus) string {
	if len(lb.Ingress) == 0 {
		return ""
	}

	if lb.Ingress[0].IP != "" {
		return lb.Ingress[0].IP
	}

	return lb.Ingress[0].Hostname
}
