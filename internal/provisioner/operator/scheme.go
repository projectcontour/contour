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

package operator

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	// scheme contains all the API types necessary for the operator's dynamic
	// clients to work. Any new non-core types must be added here.
	//
	// NOTE: The discovery mechanism used by the client doesn't automatically
	// refresh, so only add types here that are guaranteed to exist before the
	// operator starts.
	scheme = runtime.NewScheme()
)

func init() {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := gatewayv1alpha2.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

// GetOperatorScheme returns a scheme with types supported by the operator.
func GetOperatorScheme() *runtime.Scheme {
	return scheme
}
