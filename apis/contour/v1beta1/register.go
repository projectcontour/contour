// Copyright © 2019 VMware
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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName is the group name for the Contour API
	GroupName = "contour.heptio.com"
)

// SchemeGroupVersion is the GroupVersion for the Contour API
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1beta1"}
var IngressRouteGVR = SchemeGroupVersion.WithResource("ingressroutes")
var TLSCertificateDelegationGVR = SchemeGroupVersion.WithResource("tlscertificatedelegations")

// Resource gets an Contour GroupResource for a specified resource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func AddKnownTypes(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&IngressRoute{},
		&IngressRouteList{},
		&TLSCertificateDelegation{},
		&TLSCertificateDelegationList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
}
