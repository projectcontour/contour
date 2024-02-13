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
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/ingressclass"
)

// StatusAddressUpdater observes informer OnAdd and OnUpdate events and
// updates the ingress.status.loadBalancer field on all Ingress
// objects that match the ingress class (if used).
// Note that this is intended to handle updating the status.loadBalancer struct only,
// not more general status updates. That's a job for the StatusUpdater.
type StatusAddressUpdater struct {
	Logger            logrus.FieldLogger
	Cache             cache.Cache
	LBStatus          core_v1.LoadBalancerStatus
	IngressClassNames []string
	GatewayRef        *types.NamespacedName
	StatusUpdater     StatusUpdater

	// mu guards the LBStatus field, which can be updated dynamically.
	mu sync.Mutex
}

// Set updates the LBStatus field.
func (s *StatusAddressUpdater) Set(status core_v1.LoadBalancerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LBStatus = status
}

// OnAdd updates the given Ingress/HTTPProxy/Gateway object with the
// current load balancer address. Note that this method can be called
// concurrently from an informer or from Contour itself.
func (s *StatusAddressUpdater) OnAdd(obj any, _ bool) {
	// Hold the mutex to get a shallow copy. We don't need to
	// deep copy, since all the references are read-only.
	s.mu.Lock()
	loadBalancerStatus := s.LBStatus
	s.mu.Unlock()

	// Do nothing if we don't have any addresses to set.
	if len(loadBalancerStatus.Ingress) == 0 {
		return
	}

	logNoMatch := func(logger logrus.FieldLogger, obj meta_v1.Object) {
		logger.WithField("name", obj.GetName()).
			WithField("namespace", obj.GetNamespace()).
			WithField("ingress-class-annotation", annotation.IngressClass(obj)).
			WithField("kind", KindOf(obj)).
			WithField("target-ingress-classes", s.IngressClassNames).
			Debug("unmatched ingress class, skipping status address update")
	}

	switch o := obj.(type) {
	case *networking_v1.Ingress:
		if !ingressclass.MatchesIngress(o, s.IngressClassNames) {
			logNoMatch(s.Logger.WithField("ingress-class-name", ptr.Deref(o.Spec.IngressClassName, "")), o)
			return
		}

		s.StatusUpdater.Send(NewStatusUpdate(
			o.Name,
			o.Namespace,
			&networking_v1.Ingress{},
			StatusMutatorFunc(func(obj client.Object) client.Object {
				ing, ok := obj.(*networking_v1.Ingress)
				if !ok {
					panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
						obj.GetName(), obj.GetNamespace(),
					))
				}

				dco := ing.DeepCopy()
				dco.Status.LoadBalancer = coreToNetworkingLBStatus(loadBalancerStatus)
				return dco
			}),
		))

	case *contour_v1.HTTPProxy:
		if !ingressclass.MatchesHTTPProxy(o, s.IngressClassNames) {
			logNoMatch(s.Logger, o)
			return
		}

		s.StatusUpdater.Send(NewStatusUpdate(
			o.Name,
			o.Namespace,
			&contour_v1.HTTPProxy{},
			StatusMutatorFunc(func(obj client.Object) client.Object {
				proxy, ok := obj.(*contour_v1.HTTPProxy)
				if !ok {
					panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
						obj.GetName(), obj.GetNamespace(),
					))
				}

				dco := proxy.DeepCopy()
				dco.Status.LoadBalancer = loadBalancerStatus
				return dco
			}),
		))

	case *gatewayapi_v1.Gateway:
		if s.GatewayRef != nil {
			// Specific Gateway configured: check if the added Gateway
			// matches.
			if NamespacedNameOf(o) != *s.GatewayRef {
				s.Logger.
					WithField("name", o.Name).
					WithField("namespace", o.Namespace).
					Debug("Gateway is not for this Contour, not setting address")
				return
			}
		}

		s.StatusUpdater.Send(NewStatusUpdate(
			o.Name,
			o.Namespace,
			&gatewayapi_v1.Gateway{},
			StatusMutatorFunc(func(obj client.Object) client.Object {
				gateway, ok := obj.(*gatewayapi_v1.Gateway)
				if !ok {
					panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
						obj.GetName(), obj.GetNamespace(),
					))
				}

				dco := gateway.DeepCopy()
				dco.Status.Addresses = lbStatusToGatewayAddresses(loadBalancerStatus)
				return dco
			}),
		))

	default:
		s.Logger.Debugf("unsupported type %T received", o)
		return
	}
}

func (s *StatusAddressUpdater) OnUpdate(_, newObj any) {
	// We only care about the new object, because we're only updating its status.
	// So, we can get away with just passing this call to OnAdd.
	s.OnAdd(newObj, false)
}

func (s *StatusAddressUpdater) OnDelete(_ any) {
	// we don't need to update the status on resources that
	// have been deleted.
}

// ServiceStatusLoadBalancerWatcher implements ResourceEventHandler and
// watches for changes to the status.loadbalancer field
// Note that we specifically *don't* inspect inside the struct, as sending empty values
// is desirable to clear the status.
type ServiceStatusLoadBalancerWatcher struct {
	ServiceName string
	LBStatus    chan core_v1.LoadBalancerStatus
	Log         logrus.FieldLogger
}

func (s *ServiceStatusLoadBalancerWatcher) OnAdd(obj any, _ bool) {
	svc, ok := obj.(*core_v1.Service)
	if !ok {
		// not a service
		return
	}
	if svc.Name != s.ServiceName {
		return
	}
	s.Log.WithField("name", svc.Name).
		WithField("namespace", svc.Namespace).
		Debug("received new service address")

	s.notify(svc.Status.LoadBalancer)
}

func (s *ServiceStatusLoadBalancerWatcher) OnUpdate(_, newObj any) {
	svc, ok := newObj.(*core_v1.Service)
	if !ok {
		// not a service
		return
	}
	if svc.Name != s.ServiceName {
		return
	}
	s.Log.WithField("name", svc.Name).
		WithField("namespace", svc.Namespace).
		Debug("received new service address")

	s.notify(svc.Status.LoadBalancer)
}

func (s *ServiceStatusLoadBalancerWatcher) OnDelete(obj any) {
	svc, ok := obj.(*core_v1.Service)
	if !ok {
		// not a service
		return
	}
	if svc.Name != s.ServiceName {
		return
	}
	s.notify(core_v1.LoadBalancerStatus{
		Ingress: nil,
	})
}

func (s *ServiceStatusLoadBalancerWatcher) notify(lbstatus core_v1.LoadBalancerStatus) {
	s.LBStatus <- lbstatus
}

func coreToNetworkingLBStatus(lbs core_v1.LoadBalancerStatus) networking_v1.IngressLoadBalancerStatus {
	ingress := make([]networking_v1.IngressLoadBalancerIngress, len(lbs.Ingress))
	for i, ing := range lbs.Ingress {
		ports := make([]networking_v1.IngressPortStatus, len(ing.Ports))
		for j, ps := range ing.Ports {
			ports[j] = networking_v1.IngressPortStatus{
				Port:     ps.Port,
				Protocol: ps.Protocol,
				Error:    ps.Error,
			}
		}
		ingress[i] = networking_v1.IngressLoadBalancerIngress{
			IP:       ing.IP,
			Hostname: ing.Hostname,
			Ports:    ports,
		}
	}
	return networking_v1.IngressLoadBalancerStatus{
		Ingress: ingress,
	}
}

func lbStatusToGatewayAddresses(lbs core_v1.LoadBalancerStatus) []gatewayapi_v1.GatewayStatusAddress {
	addrs := []gatewayapi_v1.GatewayStatusAddress{}

	for _, lbi := range lbs.Ingress {
		if len(lbi.IP) > 0 {
			addrs = append(addrs, gatewayapi_v1.GatewayStatusAddress{
				Type:  ptr.To(gatewayapi_v1.IPAddressType),
				Value: lbi.IP,
			})
		}
		if len(lbi.Hostname) > 0 {
			addrs = append(addrs, gatewayapi_v1.GatewayStatusAddress{
				Type:  ptr.To(gatewayapi_v1.HostnameAddressType),
				Value: lbi.Hostname,
			})
		}
	}

	return addrs
}
