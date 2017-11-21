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

package envoy

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/golang/protobuf/ptypes/duration"

	"github.com/heptio/contour/internal/log"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// Translator receives notifications from the Kubernetes API and translates those
// objects into additions and removals entries of Envoy gRPC objects from a cache.
type Translator struct {
	log.Logger
	ClusterCache
	ClusterLoadAssignmentCache
	VirtualHostCache
}

func (t *Translator) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		t.addService(obj)
	case *v1.Endpoints:
		t.addEndpoints(obj)
	case *v1beta1.Ingress:
		t.addIngress(obj)
	default:
		t.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) OnUpdate(_, newObj interface{}) {
	t.OnAdd(newObj)
}

func (t *Translator) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		t.removeService(obj)
	case *v1.Endpoints:
		t.removeEndpoints(obj)
	case *v1beta1.Ingress:
		t.removeIngress(obj)
	case cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		t.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) addService(svc *v1.Service) {
	t.translateService(svc)
}

func (t *Translator) addEndpoints(ep *v1.Endpoints) {
	t.translateEndpoints(ep)
}

func (t *Translator) addIngress(i *v1beta1.Ingress) {
	t.translateIngress(i)
}

const (
	nanosecond  = 1
	microsecond = 1000 * nanosecond
	millisecond = 1000 * microsecond
)

func (t *Translator) translateService(svc *v1.Service) {
	for _, p := range svc.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			config := &v2.Cluster_EdsClusterConfig{
				EdsConfig: &v2.ConfigSource{
					ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
						ApiConfigSource: &v2.ApiConfigSource{
							ApiType:     v2.ApiConfigSource_GRPC,
							ClusterName: []string{"xds_cluster"}, // hard coded by initconfig
						},
					},
				},
				ServiceName: svc.ObjectMeta.Namespace + "/" + svc.ObjectMeta.Name + "/" + p.TargetPort.String(),
			}
			if p.Name != "" {
				// service port is named, so we must generate both a cluster for the port name
				// and a cluster for the port number.
				c := v2.Cluster{
					Name:             hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, p.Name),
					Type:             v2.Cluster_EDS,
					EdsClusterConfig: config,
					ConnectTimeout: &duration.Duration{
						Nanos: 250 * millisecond,
					},
					LbPolicy: v2.Cluster_ROUND_ROBIN,
				}
				t.ClusterCache.Add(&c)
			}
			c := v2.Cluster{
				Name:             hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, strconv.Itoa(int(p.Port))),
				Type:             v2.Cluster_EDS,
				EdsClusterConfig: config,
				ConnectTimeout: &duration.Duration{
					Nanos: 250 * millisecond,
				},
				LbPolicy: v2.Cluster_ROUND_ROBIN,
			}
			t.ClusterCache.Add(&c)
		default:
			// ignore UDP and other port types.
		}

	}
}

func (t *Translator) removeService(s *v1.Service) {
	name := hashname(60, s.Namespace, s.Name)
	t.ClusterCache.Remove(name)
}

type ClusterLoadAssignmentHandler struct {
	ClusterLoadAssignmentCache
	log.Logger
}

func (t *Translator) translateEndpoints(e *v1.Endpoints) {
	for _, s := range e.Subsets {
		// skip any subsets that don't ahve ready addresses or ports
		if len(s.Addresses) == 0 || len(s.Ports) == 0 {
			continue
		}

		for _, p := range s.Ports {
			cla := v2.ClusterLoadAssignment{
				ClusterName: hashname(60, e.ObjectMeta.Namespace, e.ObjectMeta.Name, strconv.Itoa(int(p.Port))),
				Endpoints: []*v2.LocalityLbEndpoints{{
					Locality: &v2.Locality{
						Region:  "ap-southeast-2",
						Zone:    "2b",
						SubZone: "banana",
					},
				}},
				Policy: &v2.ClusterLoadAssignment_Policy{
					DropOverload: 0.0,
				},
			}

			for _, a := range s.Addresses {
				cla.Endpoints[0].LbEndpoints = append(cla.Endpoints[0].LbEndpoints, &v2.LbEndpoint{
					Endpoint: &v2.Endpoint{
						Address: &v2.Address{
							Address: &v2.Address_SocketAddress{
								SocketAddress: &v2.SocketAddress{
									Protocol: v2.SocketAddress_TCP,
									Address:  a.IP,
									PortSpecifier: &v2.SocketAddress_PortValue{
										PortValue: uint32(p.Port),
									},
								},
							},
						},
					},
				})
			}
			t.ClusterLoadAssignmentCache.Add(&cla)
		}
	}
}

func (t *Translator) removeEndpoints(e *v1.Endpoints) {
	for _, s := range e.Subsets {
		for _, p := range s.Ports {
			if p.Name != "" {
				// endpoint port is named, so we must remove the named version
				t.ClusterLoadAssignmentCache.Remove(hashname(60, e.ObjectMeta.Namespace, e.ObjectMeta.Name, p.Name))
			}
			t.ClusterLoadAssignmentCache.Remove(hashname(60, e.ObjectMeta.Namespace, e.ObjectMeta.Name, strconv.Itoa(int(p.Port))))
		}
	}
}

type IngressResourceHandler struct {
	VirtualHostCache
	log.Logger
}

func (t *Translator) translateIngress(i *v1beta1.Ingress) {
	if i.Spec.Backend != nil {
		v := v2.VirtualHost{
			Name:    hashname(60, i.Namespace, i.Name),
			Domains: []string{"*"},
			Routes: []*v2.Route{{
				Match: &v2.RouteMatch{
					PathSpecifier: &v2.RouteMatch_Prefix{
						Prefix: "/", // match all
					},
				},
				Action: &v2.Route_Route{
					Route: &v2.RouteAction{
						ClusterSpecifier: &v2.RouteAction_Cluster{
							Cluster: ingressBackendToClusterName(i, i.Spec.Backend),
						},
					},
				},
			}},
		}
		t.VirtualHostCache.Add(&v)
	}
}

func (t *Translator) removeIngress(i *v1beta1.Ingress) {
	name := hashname(60, i.Namespace, i.Name)
	t.VirtualHostCache.Remove(name)
}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// ingressBackendToClusterName renders a cluster name from an Ingress and an IngressBackend.
func ingressBackendToClusterName(i *v1beta1.Ingress, b *v1beta1.IngressBackend) string {
	return hashname(60, i.ObjectMeta.Namespace, b.ServiceName, b.ServicePort.String())
}
