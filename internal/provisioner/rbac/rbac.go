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

package rbac

// This file only contains entries for RBAC that the Provisioner needs itself directly.
// Transitive requirements, i.e. RBAC the Provisioner needs in order to be able to create
// the Contour ClusterRoles/Roles, are handled at YAML generation time by pulling in Contour's
// RBAC entries as well.

// RBAC for Gateway API.
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses;gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status;gateways/status,verbs=update
// +kubebuilder:rbac:groups=projectcontour.io,resources=contourdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// ---

// RBAC for core Contour resources to be provisioned.
// +kubebuilder:rbac:groups="",resources=secrets;services;serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=projectcontour.io,resources=contourconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;delete
// ---

// RBAC for leader election for the provisioner.
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;update,namespace=projectcontour
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;update,namespace=projectcontour
// ---

// Contour itself has leader election RBAC scoped to a single namespace, but the provisioner
// needs it for all namespaces in order to be able to create those Roles.
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;update
// ---
