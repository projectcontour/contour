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
	"sync"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ingressclass"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// StatusAddressUpdater observes informer OnAdd and OnUpdate events and
// updates the ingress.status.loadBalancer field on all Ingress
// objects that match the ingress class (if used).
// Note that this is intended to handle updating the status.loadBalancer struct only,
// not more general status updates. That's a job for the StatusUpdater.
type StatusAddressUpdater struct {
	Logger                logrus.FieldLogger
	Cache                 cache.Cache
	LBStatus              v1.LoadBalancerStatus
	IngressClassNames     []string
	GatewayControllerName string
	StatusUpdater         StatusUpdater

	// mu guards the LBStatus field, which can be updated dynamically.
	mu sync.Mutex
}

// Set updates the LBStatus field.
func (s *StatusAddressUpdater) Set(status v1.LoadBalancerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LBStatus = status
}

// OnAdd updates the given Ingress/HTTPProxy/Gateway object with the
// current load balancer address. Note that this method can be called
// concurrently from an informer or from Contour itself.
func (s *StatusAddressUpdater) OnAdd(obj interface{}) {
	// Hold the mutex to get a shallow copy. We don't need to
	// deep copy, since all the references are read-only.
	s.mu.Lock()
	loadBalancerStatus := s.LBStatus
	s.mu.Unlock()

	// Do nothing if we don't have any addresses to set.
	if len(loadBalancerStatus.Ingress) == 0 {
		return
	}

	logNoMatch := func(logger logrus.FieldLogger, obj metav1.Object) {
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
			logNoMatch(s.Logger.WithField("ingress-class-name", pointer.StringPtrDerefOr(o.Spec.IngressClassName, "")), o)
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
				dco.Status.LoadBalancer = loadBalancerStatus
				return dco
			}),
		))

	case *contour_api_v1.HTTPProxy:
		if !ingressclass.MatchesHTTPProxy(o, s.IngressClassNames) {
			logNoMatch(s.Logger, o)
			return
		}

		s.StatusUpdater.Send(NewStatusUpdate(
			o.Name,
			o.Namespace,
			&contour_api_v1.HTTPProxy{},
			StatusMutatorFunc(func(obj client.Object) client.Object {
				proxy, ok := obj.(*contour_api_v1.HTTPProxy)
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

	case *gatewayapi_v1alpha2.Gateway:
		// Check if the Gateway's class is controlled by this Contour
		gc := &gatewayapi_v1alpha2.GatewayClass{}
		if err := s.Cache.Get(context.Background(), client.ObjectKey{Name: string(o.Spec.GatewayClassName)}, gc); err != nil {
			s.Logger.
				WithField("name", o.Name).
				WithField("namespace", o.Namespace).
				WithField("gatewayclass-name", o.Spec.GatewayClassName).
				WithError(err).
				Error("error getting gateway class for gateway")
			return
		}
		if string(gc.Spec.ControllerName) != s.GatewayControllerName {
			s.Logger.
				WithField("name", o.Name).
				WithField("namespace", o.Namespace).
				WithField("gatewayclass-name", o.Spec.GatewayClassName).
				WithField("gatewayclass-controller-name", gc.Spec.ControllerName).
				Debug("Gateway's class is not controlled by this Contour, not setting address")
			return
		}

		// Only set the Gateway's address if it has a condition of "Ready: true".
		var ready bool
		for _, cond := range o.Status.Conditions {
			if cond.Type == string(gatewayapi_v1alpha2.GatewayConditionReady) && cond.Status == metav1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			s.Logger.
				WithField("name", o.Name).
				WithField("namespace", o.Namespace).
				Debug("Gateway is not ready, not setting address")
			return
		}

		s.StatusUpdater.Send(NewStatusUpdate(
			o.Name,
			o.Namespace,
			&gatewayapi_v1alpha2.Gateway{},
			StatusMutatorFunc(func(obj client.Object) client.Object {
				gateway, ok := obj.(*gatewayapi_v1alpha2.Gateway)
				if !ok {
					panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
						obj.GetName(), obj.GetNamespace(),
					))
				}

				dco := gateway.DeepCopy()

				if len(loadBalancerStatus.Ingress) == 0 {
					return dco
				}

				if ip := loadBalancerStatus.Ingress[0].IP; len(ip) > 0 {
					dco.Status.Addresses = []gatewayapi_v1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayapi_v1alpha2.IPAddressType),
							Value: ip,
						},
					}
				} else if hostname := loadBalancerStatus.Ingress[0].Hostname; len(hostname) > 0 {
					dco.Status.Addresses = []gatewayapi_v1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayapi_v1alpha2.HostnameAddressType),
							Value: hostname,
						},
					}
				}

				return dco
			}),
		))

	default:
		s.Logger.Debugf("unsupported type %T received", o)
		return
	}
}

func (s *StatusAddressUpdater) OnUpdate(oldObj, newObj interface{}) {

	// We only care about the new object, because we're only updating its status.
	// So, we can get away with just passing this call to OnAdd.
	s.OnAdd(newObj)

}

func (s *StatusAddressUpdater) OnDelete(obj interface{}) {
	// we don't need to update the status on resources that
	// have been deleted.
}

// ServiceStatusLoadBalancerWatcher implements ResourceEventHandler and
// watches for changes to the status.loadbalancer field
// Note that we specifically *don't* inspect inside the struct, as sending empty values
// is desirable to clear the status.
type ServiceStatusLoadBalancerWatcher struct {
	ServiceName string
	LBStatus    chan v1.LoadBalancerStatus
	Log         logrus.FieldLogger
}

func (s *ServiceStatusLoadBalancerWatcher) OnAdd(obj interface{}) {
	svc, ok := obj.(*v1.Service)
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

func (s *ServiceStatusLoadBalancerWatcher) OnUpdate(oldObj, newObj interface{}) {
	svc, ok := newObj.(*v1.Service)
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

func (s *ServiceStatusLoadBalancerWatcher) OnDelete(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		// not a service
		return
	}
	if svc.Name != s.ServiceName {
		return
	}
	s.notify(v1.LoadBalancerStatus{
		Ingress: nil,
	})
}

func (s *ServiceStatusLoadBalancerWatcher) notify(lbstatus v1.LoadBalancerStatus) {
	s.LBStatus <- lbstatus
}
