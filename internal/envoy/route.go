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
	"sort"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/internal/dag"
)

// RouteRoute creates a route.Route_Route for the services supplied.
// If len(services) is greater than one, the route's action will be a
// weighted cluster.
func RouteRoute(r *dag.Route) *route.Route_Route {
	ra := route.RouteAction{
		RetryPolicy:   retryPolicy(r),
		Timeout:       timeout(r),
		PrefixRewrite: r.PrefixRewrite,
		HashPolicy:    hashPolicy(r),
	}

	if r.Websocket {
		ra.UpgradeConfigs = append(ra.UpgradeConfigs,
			&route.RouteAction_UpgradeConfig{
				UpgradeType: "websocket",
			},
		)
	}

	switch len(r.Clusters) {
	case 1:
		ra.ClusterSpecifier = &route.RouteAction_Cluster{
			Cluster: Clustername(r.Clusters[0]),
		}
	default:
		ra.ClusterSpecifier = &route.RouteAction_WeightedClusters{
			WeightedClusters: weightedClusters(r.Clusters),
		}
	}
	return &route.Route_Route{
		Route: &ra,
	}
}

// hashPolicy returns a slice of hash policies iff at least one of the route's
// clusters supplied uses the `Cookie` load balancing stategy.
func hashPolicy(r *dag.Route) []*route.RouteAction_HashPolicy {
	for _, c := range r.Clusters {
		if c.LoadBalancerStrategy == "Cookie" {
			return []*route.RouteAction_HashPolicy{{
				PolicySpecifier: &route.RouteAction_HashPolicy_Cookie_{
					Cookie: &route.RouteAction_HashPolicy_Cookie{
						Name: "X-Contour-Session-Affinity",
						Ttl:  duration(0),
						Path: "/",
					},
				},
			}}
		}
	}
	return nil
}

func timeout(r *dag.Route) *time.Duration {
	if r.TimeoutPolicy == nil {
		return nil
	}

	switch r.TimeoutPolicy.Timeout {
	case 0:
		// no timeout specified
		return nil
	case -1:
		// infinite timeout, set timeout value to a pointer to zero which tells
		// envoy "infinite timeout"
		return duration(0)
	default:
		return duration(r.TimeoutPolicy.Timeout)
	}
}

func retryPolicy(r *dag.Route) *route.RetryPolicy {
	if r.RetryPolicy == nil {
		return nil
	}
	if r.RetryPolicy.RetryOn == "" {
		return nil
	}

	rp := &route.RetryPolicy{
		RetryOn: r.RetryPolicy.RetryOn,
	}
	if r.RetryPolicy.NumRetries > 0 {
		rp.NumRetries = u32(r.RetryPolicy.NumRetries)
	}
	if r.RetryPolicy.PerTryTimeout > 0 {
		timeout := r.RetryPolicy.PerTryTimeout
		rp.PerTryTimeout = &timeout
	}
	return rp
}

// UpgradeHTTPS returns a route Action that redirects the request to HTTPS.
func UpgradeHTTPS() *route.Route_Redirect {
	return &route.Route_Redirect{
		Redirect: &route.RedirectAction{
			SchemeRewriteSpecifier: &route.RedirectAction_HttpsRedirect{
				HttpsRedirect: true,
			},
		},
	}
}

// RouteHeaders returns a list of headers to be applied at the Route level on envoy
func RouteHeaders() []*core.HeaderValueOption {
	return headers(
		appendHeader("x-request-start", "t=%START_TIME(%s.%3f)%"),
	)
}

// weightedClusters returns a route.WeightedCluster for multiple services.
func weightedClusters(clusters []*dag.Cluster) *route.WeightedCluster {
	var wc route.WeightedCluster
	var total int
	for _, cluster := range clusters {
		total += cluster.Weight
		wc.Clusters = append(wc.Clusters, &route.WeightedCluster_ClusterWeight{
			Name:   Clustername(cluster),
			Weight: u32(cluster.Weight),
		})
	}
	// Check if no weights were defined, if not default to even distribution
	if total == 0 {
		for _, c := range wc.Clusters {
			c.Weight.Value = 1
		}
		total = len(clusters)
	}
	wc.TotalWeight = u32(total)

	sort.Stable(clusterWeightByName(wc.Clusters))
	return &wc
}

// RouteRegex returns a regex matcher.
func RouteRegex(regex string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Regex{
			Regex: regex,
		},
	}
}

// RoutePrefix returns a prefix matcher.
func RoutePrefix(prefix string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

// VirtualHost creates a new route.VirtualHost.
func VirtualHost(hostname string) route.VirtualHost {
	domains := []string{hostname}
	if hostname != "*" {
		domains = append(domains, hostname+":*")
	}
	return route.VirtualHost{
		Name:    hashname(60, hostname),
		Domains: domains,
	}
}

type clusterWeightByName []*route.WeightedCluster_ClusterWeight

func (c clusterWeightByName) Len() int      { return len(c) }
func (c clusterWeightByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterWeightByName) Less(i, j int) bool {
	if c[i].Name == c[j].Name {
		return c[i].Weight.Value < c[j].Weight.Value
	}
	return c[i].Name < c[j].Name

}

func headers(first *core.HeaderValueOption, rest ...*core.HeaderValueOption) []*core.HeaderValueOption {
	return append([]*core.HeaderValueOption{first}, rest...)
}

func appendHeader(key, value string) *core.HeaderValueOption {
	return &core.HeaderValueOption{
		Header: &core.HeaderValue{
			Key:   key,
			Value: value,
		},
		Append: bv(true),
	}
}

func u32(val int) *types.UInt32Value { return &types.UInt32Value{Value: uint32(val)} }

var bvTrue = types.BoolValue{Value: true}

// bv returns a pointer to a true types.BoolValue if val is true,
// otherwise it returns nil.
func bv(val bool) *types.BoolValue {
	if val {
		return &bvTrue
	}
	return nil
}

func duration(d time.Duration) *time.Duration { return &d }
