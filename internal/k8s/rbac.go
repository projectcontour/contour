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

// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses/status,verbs=create;get;update

// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies;tlscertificatedelegations;extensionservices;contourconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups="projectcontour.io",resources=httpproxies/status;extensionservices/status;contourconfigurations/status,verbs=create;get;update

// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses;gateways;httproutes;tlsroutes;grpcroutes;tcproutes;referencegrants;backendtlspolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses/status;gateways/status;httproutes/status;tlsroutes/status;grpcroutes/status;tcproutes/status;backendtlspolicies/status,verbs=update

// +kubebuilder:rbac:groups="",resources=secrets;endpoints;services;namespaces;configmaps,verbs=get;list;watch

// Add RBAC policy to support leader election.
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;update,namespace=projectcontour
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;update,namespace=projectcontour

// Add RBAC policy for endpoint slices
// +kubebuilder:rbac:groups="discovery.k8s.io",resources=endpointslices,verbs=list;get;watch
