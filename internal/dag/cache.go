// Copyright © 2019 VMware
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

package dag

import (
	"bytes"
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/tools/cache"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/sirupsen/logrus"
)

const DEFAULT_INGRESS_CLASS = "contour"

// A KubernetesCache holds Kubernetes objects and associated configuration and produces
// DAG values.
type KubernetesCache struct {
	// RootNamespaces specifies the namespaces where root
	// IngressRoutes can be defined. If empty, roots can be defined in any
	// namespace.
	RootNamespaces []string

	// Contour's IngressClass.
	// If not set, defaults to DEFAULT_INGRESS_CLASS.
	IngressClass string

	ingresses            map[Meta]*v1beta1.Ingress
	ingressroutes        map[Meta]*ingressroutev1.IngressRoute
	httpproxies          map[Meta]*projectcontour.HTTPProxy
	secrets              map[Meta]*v1.Secret
	irdelegations        map[Meta]*ingressroutev1.TLSCertificateDelegation
	httpproxydelegations map[Meta]*projectcontour.TLSCertificateDelegation
	services             map[Meta]*v1.Service

	logrus.FieldLogger
}

// Meta holds the name and namespace of a Kubernetes object.
type Meta struct {
	name, namespace string
}

func toMeta(obj Object) Meta {
	m := obj.GetObjectMeta()
	return Meta{
		name:      m.GetName(),
		namespace: m.GetNamespace(),
	}
}

// Insert inserts obj into the KubernetesCache.
// Insert returns true if the cache accepted the object, or false if the value
// is not interesting to the cache. If an object with a matching type, name,
// and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj interface{}) bool {
	switch obj := obj.(type) {
	case *v1.Secret:
		valid, err := isValidSecret(obj)
		if !valid {
			if err != nil {
				om := obj.GetObjectMeta()
				kc.WithField("name", om.GetName()).
					WithField("namespace", om.GetNamespace()).
					WithField("kind", "Secret").
					WithField("version", "v1").
					Error(err)
			}
			return false
		}

		m := toMeta(obj)
		if kc.secrets == nil {
			kc.secrets = make(map[Meta]*v1.Secret)
		}
		kc.secrets[m] = obj
		return kc.secretTriggersRebuild(obj)
	case *v1.Service:
		m := toMeta(obj)
		if kc.services == nil {
			kc.services = make(map[Meta]*v1.Service)
		}
		kc.services[m] = obj
		return kc.serviceTriggersRebuild(obj)
	case *v1beta1.Ingress:
		class := ingressClass(obj)
		if class != "" && class != kc.ingressClass() {
			return false
		}
		m := toMeta(obj)
		if kc.ingresses == nil {
			kc.ingresses = make(map[Meta]*v1beta1.Ingress)
		}
		kc.ingresses[m] = obj
		return true
	case *extensionsv1beta1.Ingress:
		ingress := new(v1beta1.Ingress)
		if err := transposeIngress(obj, ingress); err != nil {
			om := obj.GetObjectMeta()
			kc.WithField("name", om.GetName()).
				WithField("namespace", om.GetNamespace()).
				Error(err)
			return false
		}
		return kc.Insert(ingress)
	case *ingressroutev1.IngressRoute:
		class := ingressClass(obj)
		if class != "" && class != kc.ingressClass() {
			return false
		}
		m := toMeta(obj)
		if kc.ingressroutes == nil {
			kc.ingressroutes = make(map[Meta]*ingressroutev1.IngressRoute)
		}
		kc.ingressroutes[m] = obj
		return true
	case *projectcontour.HTTPProxy:
		class := ingressClass(obj)
		if class != "" && class != kc.ingressClass() {
			return false
		}
		m := toMeta(obj)
		if kc.httpproxies == nil {
			kc.httpproxies = make(map[Meta]*projectcontour.HTTPProxy)
		}
		kc.httpproxies[m] = obj
		return true
	case *ingressroutev1.TLSCertificateDelegation:
		m := toMeta(obj)
		if kc.irdelegations == nil {
			kc.irdelegations = make(map[Meta]*ingressroutev1.TLSCertificateDelegation)
		}
		kc.irdelegations[m] = obj
		return true
	case *projectcontour.TLSCertificateDelegation:
		m := toMeta(obj)
		if kc.httpproxydelegations == nil {
			kc.httpproxydelegations = make(map[Meta]*projectcontour.TLSCertificateDelegation)
		}
		kc.httpproxydelegations[m] = obj
		return true

	default:
		// not an interesting object
		kc.WithField("object", obj).Error("insert unknown object")
		return false
	}
}

// ingressClass returns the IngressClass
// or DEFAULT_INGRESS_CLASS if not configured.
func (kc *KubernetesCache) ingressClass() string {
	return stringOrDefault(kc.IngressClass, DEFAULT_INGRESS_CLASS)
}

// Remove removes obj from the KubernetesCache.
// Remove returns a boolean indicating if the cache changed after the remove operation.
func (kc *KubernetesCache) Remove(obj interface{}) bool {
	switch obj := obj.(type) {
	default:
		return kc.remove(obj)
	case cache.DeletedFinalStateUnknown:
		return kc.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (kc *KubernetesCache) remove(obj interface{}) bool {
	switch obj := obj.(type) {
	case *v1.Secret:
		m := toMeta(obj)
		_, ok := kc.secrets[m]
		delete(kc.secrets, m)
		return ok
	case *v1.Service:
		m := toMeta(obj)
		_, ok := kc.services[m]
		delete(kc.services, m)
		return ok
	case *v1beta1.Ingress:
		m := toMeta(obj)
		_, ok := kc.ingresses[m]
		delete(kc.ingresses, m)
		return ok
	case *extensionsv1beta1.Ingress:
		m := toMeta(obj)
		_, ok := kc.ingresses[m]
		delete(kc.ingresses, m)
		return ok
	case *ingressroutev1.IngressRoute:
		m := toMeta(obj)
		_, ok := kc.ingressroutes[m]
		delete(kc.ingressroutes, m)
		return ok
	case *projectcontour.HTTPProxy:
		m := toMeta(obj)
		_, ok := kc.httpproxies[m]
		delete(kc.httpproxies, m)
		return ok
	case *ingressroutev1.TLSCertificateDelegation:
		m := toMeta(obj)
		_, ok := kc.irdelegations[m]
		delete(kc.irdelegations, m)
		return ok
	case *projectcontour.TLSCertificateDelegation:
		m := toMeta(obj)
		_, ok := kc.httpproxydelegations[m]
		delete(kc.httpproxydelegations, m)
		return ok
	default:
		// not interesting
		kc.WithField("object", obj).Error("remove unknown object")
		return false
	}
}

// serviceTriggersRebuild returns true if this service is referenced
// by an Ingress or IngressRoute in this cache.
func (kc *KubernetesCache) serviceTriggersRebuild(service *v1.Service) bool {
	for _, ingress := range kc.ingresses {
		if ingress.Namespace != service.Namespace {
			continue
		}
		if backend := ingress.Spec.Backend; backend != nil {
			if backend.ServiceName == service.Name {
				return true
			}
		}

		for _, rule := range ingress.Spec.Rules {
			http := rule.IngressRuleValue.HTTP
			if http == nil {
				continue
			}
			for _, path := range http.Paths {
				if path.Backend.ServiceName == service.Name {
					return true
				}
			}
		}
	}

	for _, ir := range kc.ingressroutes {
		if ir.Namespace != service.Namespace {
			continue
		}
		for _, route := range ir.Spec.Routes {
			for _, s := range route.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
		if tcpproxy := ir.Spec.TCPProxy; tcpproxy != nil {
			for _, s := range tcpproxy.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
	}

	for _, ir := range kc.httpproxies {
		if ir.Namespace != service.Namespace {
			continue
		}
		for _, route := range ir.Spec.Routes {
			for _, s := range route.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
		if tcpproxy := ir.Spec.TCPProxy; tcpproxy != nil {
			for _, s := range tcpproxy.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
	}

	return false
}

// secretTriggersRebuild returns true if this secret is referenced by an Ingress
// or IngressRoute object in this cache. If the secret is not in the same namespace
// it must be mentioned by a TLSCertificateDelegation.
func (kc *KubernetesCache) secretTriggersRebuild(secret *v1.Secret) bool {
	if _, isCA := secret.Data["ca.crt"]; isCA {
		// locating a secret validation usage involves traversing each
		// ingressroute object, determining if there is a valid delegation,
		// and if the reference the secret as a certificate. The DAG already
		// does this so don't reproduce the logic and just assume for the moment
		// that any change to a CA secret will trigger a rebuild.
		return true
	}

	delegations := make(map[string]bool) // targetnamespace/secretname to bool

	// merge ingressroute.TLSCertificateDelegation and projectcontour.TLSCertificateDelegation.
	for _, d := range kc.irdelegations {
		for _, cd := range d.Spec.Delegations {
			for _, n := range cd.TargetNamespaces {
				delegations[n+"/"+cd.SecretName] = true
			}
		}
	}
	for _, d := range kc.httpproxydelegations {
		for _, cd := range d.Spec.Delegations {
			for _, n := range cd.TargetNamespaces {
				delegations[n+"/"+cd.SecretName] = true
			}
		}
	}

	for _, ingress := range kc.ingresses {
		if ingress.Namespace == secret.Namespace {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Name {
					return true
				}
			}
		}
		if delegations[ingress.Namespace+"/"+secret.Name] {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Namespace+"/"+secret.Name {
					return true
				}
			}
		}

		if delegations["*/"+secret.Name] {
			for _, tls := range ingress.Spec.TLS {
				if tls.SecretName == secret.Namespace+"/"+secret.Name {
					return true
				}
			}
		}
	}

	for _, ir := range kc.ingressroutes {
		vh := ir.Spec.VirtualHost
		if vh == nil {
			// not a root ingress
			continue
		}
		tls := vh.TLS
		if tls == nil {
			// no tls spec
			continue
		}

		if ir.Namespace == secret.Namespace && tls.SecretName == secret.Name {
			return true
		}
		if delegations[ir.Namespace+"/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
		if delegations["*/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
	}

	for _, proxy := range kc.httpproxies {
		vh := proxy.Spec.VirtualHost
		if vh == nil {
			// not a root ingress
			continue
		}
		tls := vh.TLS
		if tls == nil {
			// no tls spec
			continue
		}

		if proxy.Namespace == secret.Namespace && tls.SecretName == secret.Name {
			return true
		}
		if delegations[proxy.Namespace+"/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
		if delegations["*/"+secret.Name] {
			if tls.SecretName == secret.Namespace+"/"+secret.Name {
				return true
			}
		}
	}

	return false
}

// transposeIngress transposes extensionis/v1beta1.Ingress objects into
// networking/v1beta1.Ingress objects.
func transposeIngress(src *extensionsv1beta1.Ingress, dst *v1beta1.Ingress) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(src); err != nil {
		return nil
	}
	dec := json.NewDecoder(&buf)
	return dec.Decode(dst)
}
