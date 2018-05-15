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
	"strings"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/sirupsen/logrus"

	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_cache "k8s.io/client-go/tools/cache"
)

const DEFAULT_INGRESS_CLASS = "contour"

type metadata struct {
	name, namespace string
}

// Translator receives notifications from the Kubernetes API and translates those
// objects into additions and removals entries of Envoy gRPC objects from a cache.
type Translator struct {
	// The logger for this Translator. There is no valid default, this value
	// must be supplied by the caller.
	logrus.FieldLogger

	ClusterCache
	ClusterLoadAssignmentCache
	ListenerCache
	VirtualHostCache

	// Contour's IngressClass.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClass string

	cache translatorCache
}

func (t *Translator) OnAdd(obj interface{}) {
	t.cache.OnAdd(obj)
	switch obj := obj.(type) {
	case *v1.Service:
		t.addService(obj)
	case *v1.Endpoints:
		t.addEndpoints(obj)
	case *v1beta1.Ingress:
		t.addIngress(obj)
		t.VirtualHostCache.Notify()
	case *v1.Secret:
		t.addSecret(obj)
	case *ingressroutev1.IngressRoute:
		t.addIngressRoute(obj)
	default:
		t.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) OnUpdate(oldObj, newObj interface{}) {
	t.cache.OnUpdate(oldObj, newObj)
	// TODO(dfc) need to inspect oldObj and remove unused parts of the config from the cache.
	switch newObj := newObj.(type) {
	case *v1.Service:
		oldObj, ok := oldObj.(*v1.Service)
		if !ok {
			t.Errorf("OnUpdate service %#v received invalid oldObj %T: %#v", newObj, oldObj, oldObj)
			return
		}
		t.updateService(oldObj, newObj)
	case *v1.Endpoints:
		oldObj, ok := oldObj.(*v1.Endpoints)
		if !ok {
			t.Errorf("OnUpdate endpoints %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
		t.updateEndpoints(oldObj, newObj)
	case *v1beta1.Ingress:
		oldObj, ok := oldObj.(*v1beta1.Ingress)
		if !ok {
			t.Errorf("OnUpdate endpoints %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
		t.updateIngress(oldObj, newObj)
		t.VirtualHostCache.Notify()
	case *v1.Secret:
		t.addSecret(newObj)
	case *ingressroutev1.IngressRoute:
		oldObj, ok := oldObj.(*ingressroutev1.IngressRoute)
		if !ok {
			t.Errorf("OnUpdate ingressRoute %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
		t.updateIngressRoute(oldObj, newObj)
		t.VirtualHostCache.Notify()
	default:
		t.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (t *Translator) OnDelete(obj interface{}) {
	t.cache.OnDelete(obj)
	switch obj := obj.(type) {
	case *v1.Service:
		t.removeService(obj)
	case *v1.Endpoints:
		t.removeEndpoints(obj)
	case *v1beta1.Ingress:
		t.removeIngress(obj)
		t.VirtualHostCache.Notify()
	case *v1.Secret:
		t.removeSecret(obj)
	case _cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	case *ingressroutev1.IngressRoute:
		t.removeIngressRoute(obj)
	default:
		t.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func (t *Translator) addService(svc *v1.Service) {
	t.recomputeService(nil, svc)
}

func (t *Translator) updateService(oldsvc, newsvc *v1.Service) {
	t.recomputeService(oldsvc, newsvc)
}

func (t *Translator) removeService(svc *v1.Service) {
	t.recomputeService(svc, nil)
}

func (t *Translator) addEndpoints(e *v1.Endpoints) {
	t.recomputeClusterLoadAssignment(nil, e)
}

func (t *Translator) updateEndpoints(oldep, newep *v1.Endpoints) {
	if len(newep.Subsets) < 1 {
		// if there are no endpoints in this object, ignore it
		// to avoid sending a noop notification to watchers.
		return
	}
	t.recomputeClusterLoadAssignment(oldep, newep)
}

func (t *Translator) removeEndpoints(e *v1.Endpoints) {
	t.recomputeClusterLoadAssignment(e, nil)
}

// ingressClass returns the IngressClass
// or DEFAULT_INGRESS_CLASS if not configured.
func (t *Translator) ingressClass() string {
	if t.IngressClass != "" {
		return t.IngressClass
	}
	return DEFAULT_INGRESS_CLASS
}

func (t *Translator) addIngress(i *v1beta1.Ingress) {
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != t.ingressClass() {
		// if there is an ingress class set, but it is not set to configured
		// or default ingress class, ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return
	}

	t.recomputeListeners(t.cache.ingresses, t.cache.secrets)

	// handle the special case of the default ingress first.
	if i.Spec.Backend != nil {
		// update t.vhosts cache
		t.recomputevhost("*", t.cache.vhosts["*"])
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			host = "*"
		}
		t.recomputevhost(host, t.cache.vhosts[host])
	}
}

func (t *Translator) updateIngress(oldIng, newIng *v1beta1.Ingress) {
	t.removeIngress(oldIng)
	t.addIngress(newIng)
}

func (t *Translator) removeIngress(i *v1beta1.Ingress) {
	class, ok := i.Annotations["kubernetes.io/ingress.class"]
	if ok && class != t.ingressClass() {
		// if there is an ingress class set, but it is not set to configured
		// or default ingress class, ignore this ingress.
		// TODO(dfc) we should also skip creating any cluster backends,
		// but this is hard to do at the moment because cds and rds are
		// independent.
		return
	}

	t.recomputeListeners(t.cache.ingresses, t.cache.secrets)

	if i.Spec.Backend != nil {
		t.recomputevhost("*", nil)
	}

	for _, rule := range i.Spec.Rules {
		host := rule.Host
		if host == "" {
			// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
			host = "*"
		}
		t.recomputevhost(rule.Host, t.cache.vhosts[host])
	}
}

func (t *Translator) addSecret(s *v1.Secret) {
	t.recomputeTLSListener(t.cache.ingresses, t.cache.secrets)
}

func (t *Translator) removeSecret(s *v1.Secret) {
	t.recomputeTLSListener(t.cache.ingresses, t.cache.secrets)
}

func (t *Translator) addIngressRoute(r *ingressroutev1.IngressRoute) {

	t.recomputeListenersIngressRoute(t.cache.routes, t.cache.secrets)

	// notify watchers that the vhost cache has probably changed.
	defer t.VirtualHostCache.Notify()

	host := r.Spec.VirtualHost.Fqdn
	if host == "" {
		// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
		host = "*"
	}

	t.recomputevhostIngressRoute(host, t.cache.vhostroutes[host])
}

func (t *Translator) removeIngressRoute(r *ingressroutev1.IngressRoute) {

	defer t.VirtualHostCache.Notify()

	t.recomputeListenersIngressRoute(t.cache.routes, t.cache.secrets)

	host := r.Spec.VirtualHost.Fqdn
	if host == "" {
		// If the host is unspecified, the Ingress routes all traffic based on the specified IngressRuleValue.
		host = "*"
	}

	t.recomputevhostIngressRoute(host, t.cache.vhostroutes[host])
}

func (t *Translator) updateIngressRoute(oldIng, newIng *ingressroutev1.IngressRoute) {
	t.removeIngressRoute(oldIng)
	t.addIngressRoute(newIng)
}

type translatorCache struct {
	ingresses map[metadata]*v1beta1.Ingress
	endpoints map[metadata]*v1.Endpoints
	services  map[metadata]*v1.Service
	routes    map[metadata]*ingressroutev1.IngressRoute

	// secrets stores tls secrets
	secrets map[metadata]*v1.Secret

	// vhosts stores a slice of vhosts with the ingress objects that
	// went into creating them.
	vhosts map[string]map[metadata]*v1beta1.Ingress

	// ingressroutes stores a slice of IngressRoutes with the routes that
	// went into creating them.
	vhostroutes map[string]map[metadata]*ingressroutev1.IngressRoute
}

func (t *translatorCache) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		if t.services == nil {
			t.services = make(map[metadata]*v1.Service)
		}
		t.services[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	case *v1.Endpoints:
		if t.endpoints == nil {
			t.endpoints = make(map[metadata]*v1.Endpoints)
		}
		t.endpoints[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	case *v1beta1.Ingress:
		if t.ingresses == nil {
			t.ingresses = make(map[metadata]*v1beta1.Ingress)
		}
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		t.ingresses[md] = obj
		if t.vhosts == nil {
			t.vhosts = make(map[string]map[metadata]*v1beta1.Ingress)
		}
		if obj.Spec.Backend != nil {
			if _, ok := t.vhosts["*"]; !ok {
				t.vhosts["*"] = make(map[metadata]*v1beta1.Ingress)
			}
			t.vhosts["*"][md] = obj
		}
		for _, rule := range obj.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			if _, ok := t.vhosts[host]; !ok {
				t.vhosts[host] = make(map[metadata]*v1beta1.Ingress)
			}
			t.vhosts[host][md] = obj
		}
	case *ingressroutev1.IngressRoute:
		if t.routes == nil {
			t.routes = make(map[metadata]*ingressroutev1.IngressRoute)
		}
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		t.routes[md] = obj
		if t.vhostroutes == nil {
			t.vhostroutes = make(map[string]map[metadata]*ingressroutev1.IngressRoute)
		}

		host := obj.Spec.VirtualHost.Fqdn
		if host == "" {
			host = "*"
		}
		if _, ok := t.vhostroutes[host]; !ok {
			t.vhostroutes[host] = make(map[metadata]*ingressroutev1.IngressRoute)
		}
		t.vhostroutes[host][md] = obj
	case *v1.Secret:
		if t.secrets == nil {
			t.secrets = make(map[metadata]*v1.Secret)
		}
		t.secrets[metadata{name: obj.Name, namespace: obj.Namespace}] = obj
	default:
		// ignore
	}
}

func (t *translatorCache) OnUpdate(oldObj, newObj interface{}) {
	switch oldObj := oldObj.(type) {
	case *v1beta1.Ingress:
		// ingress objects are special because their contents can change
		// which affects the t.vhost cache. The simplest way is to model
		// update as delete, then add.
		t.OnDelete(oldObj)
	case *ingressroutev1.IngressRoute:
		t.OnDelete(oldObj)
	}
	t.OnAdd(newObj)
}

func (t *translatorCache) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Service:
		delete(t.services, metadata{name: obj.Name, namespace: obj.Namespace})
	case *v1.Endpoints:
		delete(t.endpoints, metadata{name: obj.Name, namespace: obj.Namespace})
	case *v1beta1.Ingress:
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		delete(t.ingresses, md)
		delete(t.vhosts["*"], md)
		for _, rule := range obj.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			delete(t.vhosts[host], md)
			if len(t.vhosts[host]) == 0 {
				delete(t.vhosts, host)
			}
		}
		if len(t.vhosts["*"]) == 0 {
			delete(t.vhosts, "*")
		}
	case *ingressroutev1.IngressRoute:
		md := metadata{name: obj.Name, namespace: obj.Namespace}
		delete(t.routes, md)
		host := obj.Spec.VirtualHost.Fqdn
		if host == "" {
			host = "*"
		}
		delete(t.vhostroutes[host], md)
		if len(t.vhostroutes[host]) == 0 {
			delete(t.vhostroutes, host)
		}

		if len(t.vhostroutes["*"]) == 0 {
			delete(t.vhostroutes, "*")
		}
	case *v1.Secret:
		delete(t.secrets, metadata{name: obj.Name, namespace: obj.Namespace})
	case _cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		// ignore
	}
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

func apiconfigsource(clusters ...string) *core.ConfigSource {
	return &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType:      core.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}

// servicename returns a fixed name for this service and portname
func servicename(meta metav1.ObjectMeta, portname string) string {
	sn := []string{
		meta.Namespace,
		meta.Name,
		"",
	}[:2]
	if portname != "" {
		sn = append(sn, portname)
	}
	return strings.Join(sn, "/")
}
