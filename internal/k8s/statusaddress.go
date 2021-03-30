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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	ingress_validation "github.com/projectcontour/contour/internal/validation/ingress"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
)

// StatusAddressUpdater observes informer OnAdd and OnUpdate events and
// updates the ingress.status.loadBalancer field on all Ingress
// objects that match the ingress class (if used).
// Note that this is intended to handle updating the status.loadBalancer struct only,
// not more general status updates. That's a job for the StatusUpdater.
type StatusAddressUpdater struct {
	Logger           logrus.FieldLogger
	LBStatus         v1.LoadBalancerStatus
	IngressClassName string
	StatusUpdater    StatusUpdater
	Converter        Converter

	// mu guards the LBStatus field, which can be updated dynamically.
	mu sync.Mutex
}

// Set updates the LBStatus field.
func (s *StatusAddressUpdater) Set(status v1.LoadBalancerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LBStatus = status
}

// OnAdd updates the given Ingress or HTTPProxy object with the
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

	obj, err := s.Converter.FromUnstructured(obj)
	if err != nil {
		s.Logger.Error("unable to convert object from Unstructured")
		return
	}

	var typed metav1.Object
	var gvr schema.GroupVersionResource

	logNoMatch := func(logger logrus.FieldLogger, obj metav1.Object) {
		logger.WithField("name", obj.GetName()).
			WithField("namespace", obj.GetNamespace()).
			WithField("ingress-class-annotation", annotation.IngressClass(obj)).
			WithField("kind", KindOf(obj)).
			WithField("target-ingress-class", s.IngressClassName).
			Debug("unmatched ingress class, skipping status address update")
	}

	switch o := obj.(type) {
	case *networking_v1.Ingress:
		if !ingress_validation.MatchesIngressClassName(o, s.IngressClassName) {
			logNoMatch(s.Logger.WithField("ingress-class-name", pointer.StringPtrDerefOr(o.Spec.IngressClassName, "")), o)
			return
		}
		o.GetObjectKind().SetGroupVersionKind(networking_v1.SchemeGroupVersion.WithKind("ingress"))
		typed = o.DeepCopy()
		gvr = networking_v1.SchemeGroupVersion.WithResource("ingresses")
	case *contour_api_v1.HTTPProxy:
		if !annotation.MatchesIngressClass(o, s.IngressClassName) {
			logNoMatch(s.Logger, o)
			return
		}
		o.GetObjectKind().SetGroupVersionKind(contour_api_v1.SchemeGroupVersion.WithKind("httpproxy"))
		typed = o.DeepCopy()
		gvr = contour_api_v1.SchemeGroupVersion.WithResource("httpproxies")
	default:
		s.Logger.Debugf("unsupported type %T received", o)
		return
	}

	s.Logger.
		WithField("name", typed.GetName()).
		WithField("namespace", typed.GetNamespace()).
		WithField("ingress-class", annotation.IngressClass(typed)).
		WithField("kind", KindOf(obj)).
		WithField("defined-ingress-class", s.IngressClassName).
		Debug("received an object, sending status address update")

	s.StatusUpdater.Send(NewStatusUpdate(
		typed.GetName(),
		typed.GetNamespace(),
		gvr,
		StatusMutatorFunc(func(obj interface{}) interface{} {
			switch o := obj.(type) {
			case *networking_v1.Ingress:
				dco := o.DeepCopy()
				dco.Status.LoadBalancer = loadBalancerStatus
				return dco
			case *contour_api_v1.HTTPProxy:
				dco := o.DeepCopy()
				dco.Status.LoadBalancer = loadBalancerStatus
				return dco
			default:
				panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
					typed.GetName(), typed.GetNamespace(),
				))
			}
		}),
	))
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
