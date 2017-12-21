// Copyright © 2017 Heptio
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
	"sort"
	"strings"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"k8s.io/api/extensions/v1beta1"
)

// VirtualHostCache manage the contents of the gRPC RDS cache.
type VirtualHostCache struct {
	HTTP  virtualHostCache
	HTTPS virtualHostCache
	Cond
}

// recomputevhost recomputes the *v2.VirutalHost record from the list of ingresses
// supplied and the cache updated. If ingresses is empty then the *v2.VirtualHost
// record will be removed from the cache.
func (v *VirtualHostCache) recomputevhost(vhost string, ingresses []*v1beta1.Ingress) {
	switch len(ingresses) {
	case 0:
		// there are no ingresses registered with this vhost any more
		// remove the VirtualHost from the grpc cache.
		v.HTTP.Remove(hashname(60, vhost))
		v.HTTPS.Remove(hashname(60, vhost))
	default:
		// otherwise there is at least one ingress object associated with
		// this vhost, so regernate the cache record and add/overwrite the
		// grpc cache.
		vv := v2.VirtualHost{
			Name:    hashname(60, vhost),
			Domains: []string{vhost},
		}
		for _, ing := range ingresses {
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" && rule.Host != vhost {
					continue
				}
				if rule.IngressRuleValue.HTTP == nil {
					// TODO(dfc) plumb a logger in here so we can log this error.
					continue
				}
				for _, p := range rule.IngressRuleValue.HTTP.Paths {
					m := pathToRouteMatch(p)
					a := clusteraction(ingressBackendToClusterName(ing, &p.Backend))
					vv.Routes = append(vv.Routes, &v2.Route{Match: m, Action: a})
				}
			}
		}
		sort.Stable(sort.Reverse(longestRouteFirst(vv.Routes)))
		v.HTTP.Add(&vv)
		v.HTTPS.Add(&vv)
	}
}

type longestRouteFirst []*v2.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	a, ok := l[i].Match.PathSpecifier.(*v2.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	b, ok := l[j].Match.PathSpecifier.(*v2.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	return a.Prefix < b.Prefix
}

// pathToRoute converts a HTTPIngressPath to a partial v2.RouteMatch.
func pathToRouteMatch(p v1beta1.HTTPIngressPath) *v2.RouteMatch {
	if p.Path == "" {
		// If the Path is empty, the k8s spec says
		// "If unspecified, the path defaults to a catch all sending
		// traffic to the backend."
		// We map this it a catch all prefix route.
		return prefixmatch("/") // match all
	}
	// TODO(dfc) handle the case where p.Path does not start with "/"
	if strings.IndexAny(p.Path, `[(*\`) == -1 {
		// Envoy requires that regex matches match completely, wheres the
		// HTTPIngressPath.Path regex only requires a partial match. eg,
		// "/foo" matches "/" according to k8s rules, but does not match
		// according to Envoy.
		// To deal with this we handle the simple case, a Path without regex
		// characters as a Envoy prefix route.
		return prefixmatch(p.Path)
	}
	// At this point the path is a regex, which we hope is the same between k8s
	// IEEE 1003.1 POSIX regex, and Envoys Javascript regex.
	return regexmatch(p.Path)
}

// ingressBackendToClusterName renders a cluster name from an Ingress and an IngressBackend.
func ingressBackendToClusterName(i *v1beta1.Ingress, b *v1beta1.IngressBackend) string {
	return hashname(60, i.ObjectMeta.Namespace, b.ServiceName, b.ServicePort.String())
}

// prefixmatch returns a RouteMatch for the supplied prefix.
func prefixmatch(prefix string) *v2.RouteMatch {
	return &v2.RouteMatch{
		PathSpecifier: &v2.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

// regexmatch returns a RouteMatch for the supplied regex.
func regexmatch(regex string) *v2.RouteMatch {
	return &v2.RouteMatch{
		PathSpecifier: &v2.RouteMatch_Regex{
			Regex: regex,
		},
	}
}

// clusteraction returns a Route_Route action for the supplied cluster.
func clusteraction(cluster string) *v2.Route_Route {
	return &v2.Route_Route{
		Route: &v2.RouteAction{
			ClusterSpecifier: &v2.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}
