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

// Package contour contains the translation business logic that listens
// to Kubernetes ResourceEventHandler events and translates those into
// additions/deletions in caches connected to the Envoy xDS gRPC API server.
package contour

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"

	"github.com/heptio/contour/internal/log"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// NewTranslator returns a new Translator.
func NewTranslator(log log.Logger) *Translator {
	t := &Translator{
		Logger: log,
	}
	t.vhosts = make(map[string][]*v1beta1.Ingress)
	t.ingresses = make(map[metadata]*v1beta1.Ingress)
	t.secrets = make(map[metadata]*v1.Secret)
	return t
}

type metadata struct {
	name, namespace string
}

// Translator receives notifications from the Kubernetes API and translates those
// objects into additions and removals entries of Envoy gRPC objects from a cache.
type Translator struct {
	log.Logger
	ClusterCache
	ClusterLoadAssignmentCache
	ListenerCache
	VirtualHostCache

	// vhosts stores a slice of vhosts with the ingress objects that
	// went into creating them.
	vhosts map[string][]*v1beta1.Ingress

	ingresses map[metadata]*v1beta1.Ingress

	// secrets stores tls secrets
	secrets map[metadata]*v1.Secret
}

func (t *Translator) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		t.addService(obj)
	case *v1.Endpoints:
		t.addEndpoints(obj)
	case *v1beta1.Ingress:
		t.addIngress(obj)
	case *v1.Secret:
		t.addSecret(obj)
	default:
		t.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) OnUpdate(oldObj, newObj interface{}) {
	// TODO(dfc) need to inspect oldObj and remove unused parts of the config from the cache.
	switch newObj := newObj.(type) {
	case *v1.Service:
		t.addService(newObj)
	case *v1.Endpoints:
		t.addEndpoints(newObj)
	case *v1beta1.Ingress:
		t.addIngress(newObj)
	case *v1.Secret:
		t.addSecret(newObj)
	default:
		t.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (t *Translator) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		t.removeService(obj)
	case *v1.Endpoints:
		t.removeEndpoints(obj)
	case *v1beta1.Ingress:
		t.removeIngress(obj)
	case *v1.Secret:
		t.removeSecret(obj)
	case cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		t.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) addService(svc *v1.Service) {
	defer t.ClusterCache.Notify()
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
					ConnectTimeout:   250 * time.Millisecond,
					LbPolicy:         v2.Cluster_ROUND_ROBIN,
				}
				t.ClusterCache.Add(&c)
			}
			c := v2.Cluster{
				Name:             hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, strconv.Itoa(int(p.Port))),
				Type:             v2.Cluster_EDS,
				EdsClusterConfig: config,
				ConnectTimeout:   250 * time.Millisecond,
				LbPolicy:         v2.Cluster_ROUND_ROBIN,
			}
			t.ClusterCache.Add(&c)
		default:
			// ignore UDP and other port types.
		}

	}
}

func (t *Translator) removeService(svc *v1.Service) {
	defer t.ClusterCache.Notify()
	for _, p := range svc.Spec.Ports {
		switch p.Protocol {
		case "TCP":
			if p.Name != "" {
				// service port is named, so we must generate both a cluster for the port name
				// and a cluster for the port number.
				t.ClusterCache.Remove(hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, p.Name))
			}
			t.ClusterCache.Remove(hashname(60, svc.ObjectMeta.Namespace, svc.ObjectMeta.Name, strconv.Itoa(int(p.Port))))
		default:
			// ignore UDP and other port types.
		}

	}
}

func (t *Translator) addEndpoints(e *v1.Endpoints) {
	if len(e.Subsets) < 1 {
		// if there are no endpoints in this object, ignore it
		// to avoid sending a noop notification to watchers.
		return
	}
	defer t.ClusterLoadAssignmentCache.Notify()
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
	defer t.ClusterLoadAssignmentCache.Notify()
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

func (t *Translator) addIngress(i *v1beta1.Ingress) {
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != "contour" {
		// if there is an ingress class set, but it is not set to "contour"
		// ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return
	}

	t.ingresses[metadata{name: i.Name, namespace: i.Namespace}] = i

	t.recomputeListener(t.ingresses)
	if len(i.Spec.TLS) > 0 {
		t.recomputeTLSListener(t.ingresses, t.secrets)
	}

	// notify watchers that the vhost cache has probably changed.
	defer t.VirtualHostCache.Notify()

	// handle the special case of the default ingress first.
	if i.Spec.Backend != nil {
		// update t.vhosts cache
		vhosts := append(t.vhosts["*"], i)
		if len(vhosts) > 1 {
			t.Errorf("ingress %s/%s: default ingress already registered to %s/%s", i.Namespace, i.Name, vhosts[0].Namespace, vhosts[0].Name)
			return
		}
		t.vhosts["*"] = vhosts

		v := v2.VirtualHost{
			Name:    "*",
			Domains: []string{"*"},
			Routes: []*v2.Route{{
				Match:  prefixmatch("/"), // match all
				Action: clusteraction(ingressBackendToClusterName(i, i.Spec.Backend)),
			}},
		}
		t.VirtualHostCache.HTTP.Add(&v)
		return
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			host = "*"
		}
		t.vhosts[host] = appendIfMissing(t.vhosts[host], i)
		t.recomputevhost(host, t.vhosts[host])
	}
}

func (t *Translator) removeIngress(i *v1beta1.Ingress) {
	defer t.VirtualHostCache.Notify()

	delete(t.ingresses, metadata{name: i.Name, namespace: i.Namespace})

	t.recomputeListener(t.ingresses)
	if len(i.Spec.TLS) > 0 {
		t.recomputeTLSListener(t.ingresses, t.secrets)
	}

	if i.Spec.Backend != nil {
		t.VirtualHostCache.HTTP.Remove("*")
		return
	}

	for _, rule := range i.Spec.Rules {
		t.vhosts[rule.Host] = removeIfPresent(t.vhosts[rule.Host], i)
		t.recomputevhost(rule.Host, t.vhosts[rule.Host])
	}
}

func (t *Translator) addSecret(s *v1.Secret) {
	_, cert := s.Data[v1.TLSCertKey]
	_, key := s.Data[v1.TLSPrivateKeyKey]
	if !cert || !key {
		t.Logger.Infof("ignoring secret %s/%s", s.Namespace, s.Name)
		return
	}
	t.Logger.Infof("caching secret %s/%s", s.Namespace, s.Name)
	t.writeCerts(s)
	t.secrets[metadata{name: s.Name, namespace: s.Namespace}] = s

	t.recomputeTLSListener(t.ingresses, t.secrets)
}

func (t *Translator) removeSecret(s *v1.Secret) {
	delete(t.secrets, metadata{name: s.Name, namespace: s.Namespace})
	t.recomputeTLSListener(t.ingresses, t.secrets)
}

// writeSecret writes the contents of the secret to a fixed location on
// disk so that envoy can pick them up.
// TODO(dfc) this is due to https://github.com/envoyproxy/envoy/issues/1357
func (t *Translator) writeCerts(s *v1.Secret) {
	const base = "/config/ssl"
	path := filepath.Join(base, s.Namespace, s.Name)
	if err := os.MkdirAll(path, 0644); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
	if err := ioutil.WriteFile(filepath.Join(path, v1.TLSCertKey), s.Data[v1.TLSCertKey], 0755); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
	if err := ioutil.WriteFile(filepath.Join(path, v1.TLSPrivateKeyKey), s.Data[v1.TLSPrivateKeyKey], 0755); err != nil {
		t.Errorf("could not write cert %s/%s: %v", s.Namespace, s.Name, err)
		return
	}
}

func appendIfMissing(haystack []*v1beta1.Ingress, needle *v1beta1.Ingress) []*v1beta1.Ingress {
	for i := range haystack {
		if haystack[i].Name == needle.Name && haystack[i].Namespace == needle.Namespace {
			return haystack
		}
	}
	return append(haystack, needle)
}

func removeIfPresent(haystack []*v1beta1.Ingress, needle *v1beta1.Ingress) []*v1beta1.Ingress {
	for i := range haystack {
		if haystack[i].Name == needle.Name && haystack[i].Namespace == needle.Namespace {
			return append(haystack[:i], haystack[i+1:]...)
		}
	}
	return haystack
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
