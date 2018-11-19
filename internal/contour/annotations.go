// Copyright © 2018 Heptio
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

package contour

import (
	"strings"

	"github.com/gogo/protobuf/types"
	"k8s.io/api/extensions/v1beta1"
)

const (
	// set docs/annotations.md for details of how these annotations
	// are applied by Contour.

	annotationWebsocketRoutes = "contour.heptio.com/websocket-routes"
)

// httpAllowed returns true unless the kubernetes.io/ingress.allow-http annotation is
// present and set to false.
func httpAllowed(i *v1beta1.Ingress) bool {
	return !(i.Annotations["kubernetes.io/ingress.allow-http"] == "false")
}

// websocketRoutes returns a map of websocket routes. If the value is not present, or
// malformed, then an empty map is returned.
func websocketRoutes(i *v1beta1.Ingress) map[string]*types.BoolValue {
	routes := make(map[string]*types.BoolValue)
	for _, v := range strings.Split(i.Annotations[annotationWebsocketRoutes], ",") {
		route := strings.TrimSpace(v)
		if route != "" {
			routes[route] = &types.BoolValue{Value: true}
		}
	}
	return routes
}
