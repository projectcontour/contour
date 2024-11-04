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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// NewContourScheme returns a scheme that includes all the API types
// that Contour supports as well as the core Kubernetes API types from
// the default scheme.
func NewContourScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	b := runtime.SchemeBuilder{
		contour_v1.AddToScheme,
		contour_v1alpha1.AddToScheme,
		scheme.AddToScheme,
		gatewayapi_v1alpha2.Install,
		gatewayapi_v1alpha3.Install,
		gatewayapi_v1beta1.Install,
		gatewayapi_v1.Install,
	}

	if err := b.AddToScheme(s); err != nil {
		return nil, err
	}

	return s, nil
}
