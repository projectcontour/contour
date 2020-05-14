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

package k8s

import (
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
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
}

func (s *StatusAddressUpdater) OnAdd(obj interface{}) {

	ing := obj.(*v1beta1.Ingress).DeepCopy()
	ing.SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("ingress"))
	if !annotation.MatchesIngressClass(ing, s.IngressClass) {
		s.Logger.
			WithField("name", ing.GetName()).
			WithField("namespace", ing.GetNamespace()).
			WithField("ingress-class", annotation.IngressClass(ing)).
			WithField("configured-ingress-class", s.IngressClass).
			Debug("unmatched ingress class, skip status update")
		return
	}

	s.StatusUpdater.Update(
		ing.GetName(),
		ing.GetNamespace(),
		v1beta1.SchemeGroupVersion.WithResource("ingresses"),
		StatusMutatorFunc(ingressMutator(s.LBStatus)),
	)
}

func (s *StatusAddressUpdater) OnUpdate(oldObj, newObj interface{}) {

	newIng := newObj.(*v1beta1.Ingress).DeepCopy()
	newIng.SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("ingress"))
	if annotation.MatchesIngressClass(newIng, s.IngressClass) {
		// Add status because we started matching ingress-class.
		s.Logger.
			WithField("name", newIng.GetName()).
			WithField("namespace", newIng.GetNamespace()).
			WithField("ingress-class", annotation.IngressClass(newIng)).
			WithField("configured-ingress-class", s.IngressClass).
			Debug("Updated Ingress is in scope, updating")
		s.StatusUpdater.Update(
			newIng.GetName(),
			newIng.GetNamespace(),
			v1beta1.SchemeGroupVersion.WithResource("ingresses"),
			StatusMutatorFunc(ingressMutator(s.LBStatus)),
		)
	}

	// TODO(youngnick): There is a possibility that someone else may have edited the status, and we would then have
	// no way to fix the object, because we're only operating on ingress-class change. After consideration, we've decided that
	// editing the status subresource is hard enough that if someone does, they must have a reason. We can revisit if required.
	// Checking annotation.MatchesIngressClass(newIng, s.IngressClass) && !reflect.DeepEqual(newIng.Status.Loadbalancer, s.LBStatus)
	// would probably do it, but we have no way to verify for now.
}

func (s *StatusAddressUpdater) OnDelete(obj interface{}) {
	// we don't need to update the status on resources that
	// have been deleted.
}

func ingressMutator(lbstatus v1.LoadBalancerStatus) func(obj interface{}) interface{} {

	return func(obj interface{}) interface{} {
		ing := obj.(*v1beta1.Ingress).DeepCopy()
		ing.Status.LoadBalancer = lbstatus
		return ing
	}
}

// ServiceStatusLoadBalancerWatcher implements ResourceEventHandler and
// watches for changes to the status.loadbalancer field
// Note that we specifically *don't* inspect inside the struct, as sending empty values
// is desirable to clear the status.
type ServiceStatusLoadBalancerWatcher struct {
	ServiceName string
	LBStatus    chan v1.LoadBalancerStatus
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
