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
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/extensions/v1beta1"
)

// VirtualHostCache manage the contents of the gRPC RDS cache.
type VirtualHostCache struct {
	HTTP  virtualHostCache
	HTTPS virtualHostCache
	Cond
}

// recomputevhost recomputes the ingress_http (HTTP) and ingress_https (HTTPS) record
// from the vhost from list of ingresses supplied.
func (v *VirtualHostCache) recomputevhost(vhost string, ingresses map[metadata]*v1beta1.Ingress) {
	// handle ingress_https (TLS) vhost routes first.
	vv := virtualhost(vhost, "443")
	for _, ing := range ingresses {
		if !validTLSSpecforVhost(vhost, ing) {
			continue
		}
		wr := websocketRoutes(ing)
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" && rule.Host != vhost {
				continue
			}
			if rule.IngressRuleValue.HTTP == nil {
				// TODO(dfc) plumb a logger in here so we can log this error.
				continue
			}

			for _, p := range rule.IngressRuleValue.HTTP.Paths {
				vv.Routes = append(vv.Routes, route.Route{
					Match:  pathToRouteMatch(p),
					Action: action(ing, &p.Backend, wr[p.Path]),
				})
			}
		}
	}
	if len(vv.Routes) > 0 {
		sort.Stable(sort.Reverse(longestRouteFirst(vv.Routes)))
		v.HTTPS.Add(vv)
	} else {
		v.HTTPS.Remove(vv.Name)
	}

	// now handle ingress_http (non tls) routes.
	vv = virtualhost(vhost, "80")
	for _, i := range ingresses {
		if !httpAllowed(i) {
			// skip this vhosts ingress_http route.
			continue
		}
		wr := websocketRoutes(i)
		requireTLS := tlsRequired(i)
		if i.Spec.Backend != nil && vhost == "*" {
			r := route.Route{
				Match:  prefixmatch("/"),
				Action: action(i, i.Spec.Backend, wr["/"]),
			}

			if requireTLS {
				r.Action = &route.Route_Redirect{
					Redirect: &route.RedirectAction{
						HttpsRedirect: true,
					},
				}
			}
			vv.Routes = []route.Route{r}
		}
		for _, rule := range i.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}

			if host != vhost {
				// not this vhost
				continue
			}

			if rule.IngressRuleValue.HTTP == nil {
				// TODO(dfc) plumb a logger in here so we can log this error.
				continue
			}
			for _, p := range rule.IngressRuleValue.HTTP.Paths {
				r := route.Route{
					Match:  pathToRouteMatch(p),
					Action: action(i, &p.Backend, wr[p.Path]),
				}
				if requireTLS {
					r.Action = &route.Route_Redirect{
						Redirect: &route.RedirectAction{
							HttpsRedirect: true,
						},
					}
				}
				vv.Routes = append(vv.Routes, r)
			}
		}
	}
	if len(vv.Routes) > 0 {
		sort.Stable(sort.Reverse(longestRouteFirst(vv.Routes)))
		v.HTTP.Add(vv)
	} else {
		v.HTTP.Remove(vv.Name)
	}
}

// recomputevhostIngressRoute recomputes the ingress_http (HTTP) and ingress_https (HTTPS) record
// from the vhost from list of ingresses supplied.
func (v *VirtualHostCache) recomputevhostIngressRoute(vhost string, routes map[metadata]*ingressroutev1.IngressRoute) {
	// now handle ingress_http (non tls) routes.
	vv := virtualhost(vhost, "80")
	for _, i := range routes {
		for _, j := range i.Spec.Routes {
			// TODO(sas): Handle case of no default path (e.g. "/")

			vv.Routes = append(vv.Routes, route.Route{
				Match:  prefixmatch(j.Match),
				Action: actionroute(i.ObjectMeta.Namespace, j.Services),
			})
		}
	}

	if len(vv.Routes) > 0 {
		sort.Stable(sort.Reverse(longestRouteFirst(vv.Routes)))
		v.HTTP.Add(vv)
	} else {
		v.HTTP.Remove(vv.Name)
	}
}

// action computes the cluster route action, a *route.Route_route for the
// supplied ingress and backend.
func action(i *v1beta1.Ingress, be *v1beta1.IngressBackend, useWebsocket *types.BoolValue) *route.Route_Route {
	name := ingressBackendToClusterName(i.ObjectMeta.Namespace, be.ServiceName, be.ServicePort.String())
	ca := route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_Cluster{
				Cluster: name,
			},
		},
	}
	if timeout, ok := parseAnnotationTimeout(i.Annotations, annotationRequestTimeout); ok {
		ca.Route.Timeout = &timeout
	}

	if retryOn, ok := i.Annotations[annotationRetryOn]; ok {
		ca.Route.RetryPolicy = &route.RouteAction_RetryPolicy{
			RetryOn:    retryOn,
			NumRetries: parseAnnotationUInt32(i.Annotations, annotationNumRetries),
		}
		if perTryTimeout, ok := parseAnnotationTimeout(i.Annotations, annotationPerTryTimeout); ok {
			ca.Route.RetryPolicy.PerTryTimeout = &perTryTimeout
		}
	}

	ca.Route.UseWebsocket = useWebsocket

	return &ca
}

// actionroute computes the cluster route action, a *v2.Route_route for the
// supplied ingress and backend
func actionroute(namespace string, be []ingressroutev1.Service) *route.Route_Route {

	totalWeight := 100
	totalUpstreams := len(be)

	upstreams := []*route.WeightedCluster_ClusterWeight{}

	// Loop over all the upstreams and add to slice
	for _, i := range be {

		name := ingressBackendToClusterName(namespace, i.Name, strconv.Itoa(i.Port))

		// Create the empty upstream
		upstream := route.WeightedCluster_ClusterWeight{
			Name:   name,
			Weight: &types.UInt32Value{},
		}

		// If weight is passed, then use otherwise calculate
		if i.Weight != nil {
			upstream.Weight.Value = uint32(*i.Weight)
			totalWeight -= *i.Weight
			totalUpstreams--
		} else {
			upstream.Weight.Value = uint32(math.Floor(float64(totalWeight) / float64(totalUpstreams)))
		}

		upstreams = append(upstreams, &upstream)
	}

	// Create Route with slice of upstreams
	ca := route.Route_Route{
		Route: &route.RouteAction{
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: upstreams,
				},
			},
		},
	}

	// if timeout, ok := getRequestTimeout(i.Annotations); ok {
	// 	ca.Route.Timeout = &timeout
	// }
	return &ca
}

// validTLSSpecForVhost returns if this ingress object
// contains a TLS spec that matches the vhost supplied,
func validTLSSpecforVhost(vhost string, i *v1beta1.Ingress) bool {
	for _, tls := range i.Spec.TLS {
		if tls.SecretName == "" {
			// not a valid TLS spec without a secret for the cert.
			continue
		}

		for _, h := range tls.Hosts {
			if h == vhost {
				return true
			}
		}
	}
	return false
}

type longestRouteFirst []route.Route

func (l longestRouteFirst) Len() int      { return len(l) }
func (l longestRouteFirst) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l longestRouteFirst) Less(i, j int) bool {
	a, ok := l[i].Match.PathSpecifier.(*route.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	b, ok := l[j].Match.PathSpecifier.(*route.RouteMatch_Prefix)
	if !ok {
		// ignore non prefix matches
		return false
	}

	return a.Prefix < b.Prefix
}

// pathToRoute converts a HTTPIngressPath to a partial route.RouteMatch.
func pathToRouteMatch(p v1beta1.HTTPIngressPath) route.RouteMatch {
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

// ingressBackendToClusterName renders a cluster name from an namespace, servicename, & service port
func ingressBackendToClusterName(namespace, servicename, serviceport string) string {
	return hashname(60, namespace, servicename, serviceport)
}

// prefixmatch returns a RouteMatch for the supplied prefix.
func prefixmatch(prefix string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

// regexmatch returns a RouteMatch for the supplied regex.
func regexmatch(regex string) route.RouteMatch {
	return route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Regex{
			Regex: regex,
		},
	}
}

func virtualhost(hostname, hostport string) *route.VirtualHost {
	var domains []string
	if hostname != "*" {
		domains = []string{hostname, hostname + ":" + hostport}
	} else {
		domains = []string{hostname}
	}
	return &route.VirtualHost{
		Name:    hashname(60, hostname),
		Domains: domains,
	}
}
