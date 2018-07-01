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
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"k8s.io/api/extensions/v1beta1"
)

const (
	// set docs/annotations.md for details of how these annotations
	// are applied by Contour.

	annotationRequestTimeout  = "contour.heptio.com/request-timeout"
	annotationWebsocketRoutes = "contour.heptio.com/websocket-routes"

	// By default envoy applies a 15 second timeout to all backend requests.
	// The explicit value 0 turns off the timeout, implying "never time out"
	// https://www.envoyproxy.io/docs/envoy/v1.5.0/api-v2/rds.proto#routeaction
	infiniteTimeout = time.Duration(0)
)

// parseAnnotationTimeout parses the annotations map for a contour.heptio.com/request-timeout
// value. If the value is not present, false is returned and the timeout value should be
// ignored. If the value is present, but malformed, the timeout value is valid, and represents
// infinite timeout.
func parseAnnotationTimeout(annotations map[string]string, annotation string) (time.Duration, bool) {
	timeoutStr := annotations[annotationRequestTimeout]
	// Error or unspecified is interpreted as no timeout specified, use envoy defaults
	if timeoutStr == "" {
		return 0, false
	}
	// Interpret "infinity" explicitly as an infinite timeout, which envoy config
	// expects as a timeout of 0. This could be specified with the duration string
	// "0s" but want to give an explicit out for operators.
	if timeoutStr == "infinity" {
		return infiniteTimeout, true
	}

	timeoutParsed, err := time.ParseDuration(timeoutStr)
	if err != nil {
		// TODO(cmalonty) plumb a logger in here so we can log this error.
		// Assuming infinite duration is going to surprise people less for
		// a not-parseable duration than a implicit 15 second one.
		return infiniteTimeout, true
	}
	return timeoutParsed, true
}

// parseAnnotationUint32 parsers the annotation map for the supplied annotation key.
// If the value is not present, or malformed, then nil is returned.
func parseAnnotationUInt32(annotations map[string]string, annotation string) *types.UInt32Value {
	v, err := strconv.ParseUint(annotations[annotation], 10, 32)
	if err != nil {
		return nil
	}
	return &types.UInt32Value{Value: uint32(v)}
}

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
