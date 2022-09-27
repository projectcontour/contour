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

package equality

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

// DaemonsetConfigChanged checks if current and expected DaemonSet match,
// and if not, returns the updated DaemonSet resource.
func DaemonsetConfigChanged(current, expected *appsv1.DaemonSet) (*appsv1.DaemonSet, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		changed = true
		updated.Labels = expected.Labels

	}

	if !apiequality.Semantic.DeepEqual(current.Spec, expected.Spec) {
		changed = true
		updated.Spec = expected.Spec
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// DaemonSetSelectorsDiffer checks if the current and expected DaemonSet selectors differ.
func DaemonSetSelectorsDiffer(current, expected *appsv1.DaemonSet) bool {
	return !apiequality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector)
}

// DeploymentConfigChanged checks if the current and expected Deployment match
// and if not, returns true and the expected Deployment.
func DeploymentConfigChanged(current, expected *appsv1.Deployment) (*appsv1.Deployment, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		updated = expected
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec, expected.Spec) {
		updated = expected
		changed = true
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// DeploymentSelectorsDiffer checks if the current and expected Deployment selectors differ.
func DeploymentSelectorsDiffer(current, expected *appsv1.Deployment) bool {
	return !apiequality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector)
}

// ClusterIPServiceChanged checks if the spec of current and expected match and if not,
// returns true and the expected Service resource. The cluster IP is not compared
// as it's assumed to be dynamically assigned.
func ClusterIPServiceChanged(current, expected *corev1.Service) (*corev1.Service, bool) {
	changed := false
	updated := current.DeepCopy()

	// Spec can't simply be matched since clusterIP is being dynamically assigned.
	if len(current.Spec.Ports) != len(expected.Spec.Ports) {
		updated.Spec.Ports = expected.Spec.Ports
		changed = true
	} else if !apiequality.Semantic.DeepEqual(current.Spec.Ports, expected.Spec.Ports) {
		updated.Spec.Ports = expected.Spec.Ports
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector) {
		updated.Spec.Selector = expected.Spec.Selector
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.SessionAffinity, expected.Spec.SessionAffinity) {
		updated.Spec.SessionAffinity = expected.Spec.SessionAffinity
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Type, expected.Spec.Type) {
		updated.Spec.Type = expected.Spec.Type
		changed = true
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// LoadBalancerServiceChanged checks if current and expected match and if not, returns
// true and the expected Service resource. The healthCheckNodePort and a port's nodePort
// are not compared since they are dynamically assigned.
func LoadBalancerServiceChanged(current, expected *corev1.Service) (*corev1.Service, bool) {
	changed := false
	updated := current.DeepCopy()

	// Ports can't simply be matched since some fields are being dynamically assigned.
	if len(current.Spec.Ports) != len(expected.Spec.Ports) {
		updated.Spec.Ports = expected.Spec.Ports
		changed = true
	} else {
		for i, p := range current.Spec.Ports {
			if !apiequality.Semantic.DeepEqual(p.Name, expected.Spec.Ports[i].Name) {
				updated.Spec.Ports[i].Name = expected.Spec.Ports[i].Name
				changed = true
			}
			if !apiequality.Semantic.DeepEqual(p.Protocol, expected.Spec.Ports[i].Protocol) {
				updated.Spec.Ports[i].Protocol = expected.Spec.Ports[i].Protocol
				changed = true
			}
			if !apiequality.Semantic.DeepEqual(p.Port, expected.Spec.Ports[i].Port) {
				updated.Spec.Ports[i].Port = expected.Spec.Ports[i].Port
				changed = true
			}
			if !apiequality.Semantic.DeepEqual(p.TargetPort, expected.Spec.Ports[i].TargetPort) {
				updated.Spec.Ports[i].TargetPort = expected.Spec.Ports[i].TargetPort
				changed = true
			}
		}
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector) {
		updated.Spec.Selector = expected.Spec.Selector
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.ExternalTrafficPolicy, expected.Spec.ExternalTrafficPolicy) {
		updated.Spec.ExternalTrafficPolicy = expected.Spec.ExternalTrafficPolicy
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.SessionAffinity, expected.Spec.SessionAffinity) {
		updated.Spec.SessionAffinity = expected.Spec.SessionAffinity
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Type, expected.Spec.Type) {
		updated.Spec.Type = expected.Spec.Type
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Annotations, expected.Annotations) {
		updated.Annotations = expected.Annotations
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.LoadBalancerIP, expected.Spec.LoadBalancerIP) {
		updated.Spec.LoadBalancerIP = expected.Spec.LoadBalancerIP
		changed = true
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// NodePortServiceChanged checks if current and expected match and if not, returns
// true and the expected Service resource. The healthCheckNodePort is not compared
// since it's dynamically assigned.
func NodePortServiceChanged(current, expected *corev1.Service) (*corev1.Service, bool) {
	changed := false
	updated := current.DeepCopy()

	if len(current.Spec.Ports) != len(expected.Spec.Ports) {
		updated.Spec.Ports = expected.Spec.Ports
		changed = true
		return updated, changed
	}

	for i, p := range current.Spec.Ports {
		if !apiequality.Semantic.DeepEqual(p, expected.Spec.Ports[i]) {
			updated.Spec.Ports = expected.Spec.Ports
			changed = true
		}
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector) {
		updated.Spec.Selector = expected.Spec.Selector
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.ExternalTrafficPolicy, expected.Spec.ExternalTrafficPolicy) {
		updated.Spec.ExternalTrafficPolicy = expected.Spec.ExternalTrafficPolicy
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.SessionAffinity, expected.Spec.SessionAffinity) {
		updated.Spec.SessionAffinity = expected.Spec.SessionAffinity
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Type, expected.Spec.Type) {
		updated.Spec.Type = expected.Spec.Type
		changed = true
	}

	if !apiequality.Semantic.DeepEqual(current.Annotations, expected.Annotations) {
		updated.Annotations = expected.Annotations
		changed = true
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// ServiceAccountConfigChanged checks if the current and expected ServiceAccount
// match and if not, returns true and the expected ServiceAccount.
func ServiceAccountConfigChanged(current, expected *corev1.ServiceAccount) (*corev1.ServiceAccount, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		updated = expected
		changed = true
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// ClusterRoleConfigChanged checks if the current and expected ClusterRole
// match and if not, returns true and the expected ClusterRole.
func ClusterRoleConfigChanged(current, expected *rbacv1.ClusterRole) (*rbacv1.ClusterRole, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		changed = true
		updated.Labels = expected.Labels
	}

	if !apiequality.Semantic.DeepEqual(current.Rules, expected.Rules) {
		changed = true
		updated.Rules = expected.Rules
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// ClusterRoleBindingConfigChanged checks if the current and expected ClusterRoleBinding
// match and if not, returns true and the expected ClusterRoleBinding.
func ClusterRoleBindingConfigChanged(current, expected *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		changed = true
		updated.Labels = expected.Labels

	}

	if !apiequality.Semantic.DeepEqual(current.Subjects, expected.Subjects) {
		changed = true
		updated.Subjects = expected.Subjects
	}

	if !apiequality.Semantic.DeepEqual(current.RoleRef, expected.RoleRef) {
		changed = true
		updated.RoleRef = expected.RoleRef
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// RoleConfigChanged checks if the current and expected Role match
// and if not, returns true and the expected Role.
func RoleConfigChanged(current, expected *rbacv1.Role) (*rbacv1.Role, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		changed = true
		updated.Labels = expected.Labels
	}

	if !apiequality.Semantic.DeepEqual(current.Rules, expected.Rules) {
		changed = true
		updated.Rules = expected.Rules
	}

	if !changed {
		return nil, false
	}

	return updated, true
}

// RoleBindingConfigChanged checks if the current and expected RoleBinding
// match and if not, returns true and the expected RoleBinding.
func RoleBindingConfigChanged(current, expected *rbacv1.RoleBinding) (*rbacv1.RoleBinding, bool) {
	changed := false
	updated := current.DeepCopy()

	if !apiequality.Semantic.DeepEqual(current.Labels, expected.Labels) {
		changed = true
		updated.Labels = expected.Labels

	}

	if !apiequality.Semantic.DeepEqual(current.Subjects, expected.Subjects) {
		changed = true
		updated.Subjects = expected.Subjects
	}

	if !apiequality.Semantic.DeepEqual(current.RoleRef, expected.RoleRef) {
		changed = true
		updated.RoleRef = expected.RoleRef
	}

	if !changed {
		return nil, false
	}

	return updated, true
}
