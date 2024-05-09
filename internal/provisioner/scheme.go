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

package provisioner

import (
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
)

// CreateScheme returns a scheme with all the API types necessary for the gateway
// provisioner's client to work. Any new API groups must be added here.
func CreateScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	b := runtime.SchemeBuilder{
		clientgoscheme.AddToScheme,
		apiextensions_v1.AddToScheme,
		gatewayapi_v1alpha2.Install,
		gatewayapi_v1beta1.Install,
		gatewayapi_v1.Install,
		contour_v1alpha1.AddToScheme,
	}

	if err := b.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return scheme, nil
}
