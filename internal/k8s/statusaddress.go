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

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// StatusAddressUpdater observes informer OnAdd and OnUpdate events and
// updates the ingress.status.loadBalancer field on all Ingress
// objects that match the ingress class (if used).
// Note that this is intended to handle updating the status.loadBalancer struct only,
// not more general status updates. That's a job for the StatusUpdater.
type StatusAddressUpdater struct {
	Logger        logrus.FieldLogger
	LBStatus      v1.LoadBalancerStatus
	IngressClass  string
	StatusUpdater StatusUpdater
	Converter     Converter
}

func (s *StatusAddressUpdater) OnAdd(obj interface{}) {

	obj, err := s.Converter.FromUnstructured(obj)
	if err != nil {
		s.Logger.Error("unable to convert object from Unstructured")
		return
	}

	var typed Object
	var gvr schema.GroupVersionResource
	var kind string

	switch o := obj.(type) {
	case *v1beta1.Ingress:
		o.GetObjectKind().SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("ingress"))
		typed = o.DeepCopy()
		gvr = v1beta1.SchemeGroupVersion.WithResource("ingresses")
		kind = "ingress"
	case *projcontour.HTTPProxy:
		o.GetObjectKind().SetGroupVersionKind(projcontour.SchemeGroupVersion.WithKind("httpproxy"))
		typed = o.DeepCopy()
		gvr = projcontour.SchemeGroupVersion.WithResource("httpproxies")
		kind = "httpproxy"
	default:
		s.Logger.
			Debug("unsupported type received")
		return
	}

	if !annotation.MatchesIngressClass(typed, s.IngressClass) {
		s.Logger.
			WithField("name", typed.GetObjectMeta().GetName()).
			WithField("namespace", typed.GetObjectMeta().GetNamespace()).
			WithField("ingress-class", annotation.IngressClass(typed)).
			WithField("defined-ingress-class", s.IngressClass).
			WithField("kind", kind).
			Debug("unmatched ingress class, skipping status address update")
		return
	}

	s.Logger.
		WithField("name", typed.GetObjectMeta().GetName()).
		WithField("namespace", typed.GetObjectMeta().GetNamespace()).
		WithField("ingress-class", annotation.IngressClass(typed)).
		WithField("kind", kind).
		WithField("defined-ingress-class", s.IngressClass).
		Debug("received an object, sending status address update")

	s.StatusUpdater.Update(
		typed.GetObjectMeta().GetName(),
		typed.GetObjectMeta().GetNamespace(),
		gvr,
		StatusMutatorFunc(func(obj interface{}) interface{} {
			switch o := obj.(type) {
			case *v1beta1.Ingress:
				dco := o.DeepCopy()
				dco.Status.LoadBalancer = s.LBStatus
				return dco
			case *projcontour.HTTPProxy:
				dco := o.DeepCopy()
				dco.Status.LoadBalancer = s.LBStatus
				return dco
			default:
				panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
					typed.GetObjectMeta().GetName(), typed.GetObjectMeta().GetNamespace(),
				))
			}
		}),
	)
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
