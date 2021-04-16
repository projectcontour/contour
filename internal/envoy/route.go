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

package envoy

import (
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/timeout"
)

func HostReplaceHeader(hp *dag.HeadersPolicy) string {
	if hp == nil {
		return ""
	}
	return hp.HostRewrite
}

// Timeout converts a timeout.Setting to a protobuf.Duration
// that's appropriate for Envoy. In general (though there are
// exceptions), Envoy uses the following semantics:
//	- not passing a value means "use Envoy default"
//	- explicitly passing a 0 means "disable this timeout"
//	- passing a positive value uses that value
func Timeout(d timeout.Setting) *duration.Duration {
	switch {
	case d.UseDefault():
		// Don't pass a value to Envoy.
		return nil
	case d.IsDisabled():
		// Explicitly pass a 0.
		return protobuf.Duration(0)
	default:
		// Pass the duration value.
		return protobuf.Duration(d.Duration())
	}
}

// SingleSimpleCluster determines whether we can use a RouteAction_Cluster
// or must use a RouteAction_WeighedCluster to encode additional routing data.
func SingleSimpleCluster(clusters []*dag.Cluster) bool {
	// If there are multiple clusters, than we cannot simply dispatch
	// to it by name.
	if len(clusters) != 1 {
		return false
	}
	cluster := clusters[0]

	// If the target cluster performs any kind of header manipulation,
	// then we should use a WeightedCluster to encode the additional
	// configuration.
	if cluster.RequestHeadersPolicy == nil {
		// no request headers policy
	} else if len(cluster.RequestHeadersPolicy.Set) != 0 ||
		len(cluster.RequestHeadersPolicy.Add) != 0 ||
		len(cluster.RequestHeadersPolicy.Remove) != 0 {
		return false
	}
	if cluster.ResponseHeadersPolicy == nil {
		// no response headers policy
	} else if len(cluster.ResponseHeadersPolicy.Set) != 0 ||
		len(cluster.ResponseHeadersPolicy.Remove) != 0 {
		return false
	}

	return true
}
