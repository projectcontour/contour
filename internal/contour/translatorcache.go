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
	ingressroutev1 "github.com/heptio/contour/apis/contour/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	_cache "k8s.io/client-go/tools/cache"
)

// A translatorCache holds cached values for use by the translator.
// It is modeled as a cache.ResourceEventHandler so it can be composed
// with ResourceEventHandlers easily.
type translatorCache struct {
	ingresses map[metadata]*v1beta1.Ingress
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

		host := "*"
		if obj.Spec.VirtualHost != nil {
			if obj.Spec.VirtualHost.Fqdn != "" {
				host = obj.Spec.VirtualHost.Fqdn
			}
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

		host := "*"
		if obj.Spec.VirtualHost != nil {
			if obj.Spec.VirtualHost.Fqdn != "" {
				host = obj.Spec.VirtualHost.Fqdn
			}
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
