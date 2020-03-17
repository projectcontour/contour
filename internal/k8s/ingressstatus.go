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
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	clientset "k8s.io/client-go/kubernetes"
)

// StatusLoadbalancerUpdater observes informer OnAdd events and
// updates the ingress.status.loadBalancer field on all Ingress
// objects that match the ingress class (if used).
type StatusLoadBalancerUpdater struct {
	Client clientset.Interface
	Logger logrus.FieldLogger
	Status v1.LoadBalancerStatus
}

func (s *StatusLoadBalancerUpdater) OnAdd(obj interface{}) {
	ing := obj.(*v1beta1.Ingress).DeepCopy()

	// TODO(dfc) check ingress class

	ing.Status.LoadBalancer = s.Status
	_, err := s.Client.NetworkingV1beta1().Ingresses(ing.GetNamespace()).UpdateStatus(ing)
	if err != nil {
		s.Logger.
			WithField("name", ing.GetName()).
			WithField("namespace", ing.GetNamespace()).
			WithError(err).Error("unable to update status")
	}
}

func (s *StatusLoadBalancerUpdater) OnUpdate(oldObj, newObj interface{}) {
	// Ignoring OnUpdate allows us to avoid the message generated
	// from the status update.

	// TODO(dfc) handle these cases:
	// - OnUpdate transitions from an ingress class which is out of scope
	// to one in scope.
	// - OnUpdate transitions from an ingress class in scope to one out
	// of scope.
}

func (s *StatusLoadBalancerUpdater) OnDelete(obj interface{}) {
	// we don't need to update the status on resources that
	// have been deleted.
}
