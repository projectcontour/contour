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
	case *v1beta1.Ingress:
		// nothing, already cached
	case *v1.Secret:
		// nothing, already cached
	case *ingressroutev1.IngressRoute:
		// nothing, already cached
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
	case *v1beta1.Ingress:
		// nothing, already cached
	case *v1.Secret:
		// nothing, already cached
	case *ingressroutev1.IngressRoute:
		// nothing, already cached
	default:
		t.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (t *Translator) OnDelete(obj interface{}) {
	t.cache.OnDelete(obj)
	switch obj := obj.(type) {
	case *v1.Service:
		t.removeService(obj)
	case *v1beta1.Ingress:
		// nothing, already cached
	case *v1.Secret:
		// nothing, already cached
	case _cache.DeletedFinalStateUnknown:
		t.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	case *ingressroutev1.IngressRoute:
		// nothing, already cached
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
