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

package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ingressclass"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
)

// A KubernetesCache holds Kubernetes objects and associated configuration and produces
// DAG values.
type KubernetesCache struct {
	// RootNamespaces specifies the namespaces where root
	// HTTPProxies can be defined. If empty, roots can be defined in any
	// namespace.
	RootNamespaces []string

	// Names of ingress classes to cache HTTPProxies/Ingresses for. If not
	// set, objects with no ingress class or DEFAULT_INGRESS_CLASS will be
	// cached.
	IngressClassNames []string

	// ConfiguredGatewayToCache is the optional name of the specific Gateway to cache.
	// If set, only the Gateway with this namespace/name will be kept.
	ConfiguredGatewayToCache *types.NamespacedName

	// Secrets that are referred from the configuration file.
	ConfiguredSecretRefs []*types.NamespacedName

	ingresses                 map[types.NamespacedName]*networking_v1.Ingress
	httpproxies               map[types.NamespacedName]*contour_v1.HTTPProxy
	secrets                   map[types.NamespacedName]*Secret
	configmapsecrets          map[types.NamespacedName]*Secret
	tlscertificatedelegations map[types.NamespacedName]*contour_v1.TLSCertificateDelegation
	services                  map[types.NamespacedName]*core_v1.Service
	namespaces                map[string]*core_v1.Namespace
	gatewayclass              *gatewayapi_v1.GatewayClass
	gateway                   *gatewayapi_v1.Gateway
	httproutes                map[types.NamespacedName]*gatewayapi_v1.HTTPRoute
	tlsroutes                 map[types.NamespacedName]*gatewayapi_v1alpha2.TLSRoute
	grpcroutes                map[types.NamespacedName]*gatewayapi_v1.GRPCRoute
	tcproutes                 map[types.NamespacedName]*gatewayapi_v1alpha2.TCPRoute
	referencegrants           map[types.NamespacedName]*gatewayapi_v1beta1.ReferenceGrant
	backendtlspolicies        map[types.NamespacedName]*gatewayapi_v1alpha3.BackendTLSPolicy
	extensions                map[types.NamespacedName]*contour_v1alpha1.ExtensionService

	// Metrics contains Prometheus metrics.
	Metrics *metrics.Metrics

	Client client.Reader

	initialize sync.Once

	logrus.FieldLogger
}

// DelegationNotPermittedError is returned by KubernetesCache's Secret accessor methods when delegation is not set up.
type DelegationNotPermittedError struct {
	error
}

func NewDelegationNotPermittedError(err error) DelegationNotPermittedError {
	return DelegationNotPermittedError{err}
}

// init creates the internal cache storage. It is called implicitly from the public API.
func (kc *KubernetesCache) init() {
	kc.ingresses = make(map[types.NamespacedName]*networking_v1.Ingress)
	kc.httpproxies = make(map[types.NamespacedName]*contour_v1.HTTPProxy)
	kc.secrets = make(map[types.NamespacedName]*Secret)
	kc.configmapsecrets = make(map[types.NamespacedName]*Secret)
	kc.tlscertificatedelegations = make(map[types.NamespacedName]*contour_v1.TLSCertificateDelegation)
	kc.services = make(map[types.NamespacedName]*core_v1.Service)
	kc.namespaces = make(map[string]*core_v1.Namespace)
	kc.httproutes = make(map[types.NamespacedName]*gatewayapi_v1.HTTPRoute)
	kc.referencegrants = make(map[types.NamespacedName]*gatewayapi_v1beta1.ReferenceGrant)
	kc.tlsroutes = make(map[types.NamespacedName]*gatewayapi_v1alpha2.TLSRoute)
	kc.grpcroutes = make(map[types.NamespacedName]*gatewayapi_v1.GRPCRoute)
	kc.tcproutes = make(map[types.NamespacedName]*gatewayapi_v1alpha2.TCPRoute)
	kc.backendtlspolicies = make(map[types.NamespacedName]*gatewayapi_v1alpha3.BackendTLSPolicy)
	kc.extensions = make(map[types.NamespacedName]*contour_v1alpha1.ExtensionService)
}

// Insert inserts obj into the KubernetesCache.
// Insert returns true if the cache accepted the object, or false if the value
// is not interesting to the cache. If an object with a matching type, name,
// and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj any) bool {
	kc.initialize.Do(kc.init)

	maybeInsert := func(obj any) (bool, int) {
		switch obj := obj.(type) {
		case *core_v1.Secret:
			// Secret validation status is intentionally cleared, it needs
			// to be re-validated after an insert.
			kc.secrets[k8s.NamespacedNameOf(obj)] = &Secret{Object: obj}
			return kc.secretTriggersRebuild(obj), len(kc.secrets)

		case *core_v1.ConfigMap:
			// Only insert configmaps that are CA certs, i.e has 'ca.crt' key,
			// into cache.
			if secret, isCA := kc.convertCACertConfigMapToSecret(obj); isCA {
				kc.configmapsecrets[k8s.NamespacedNameOf(obj)] = &Secret{Object: secret}
				return kc.configMapTriggersRebuild(obj), len(kc.configmapsecrets)
			}
			return false, len(kc.configmapsecrets)

		case *core_v1.Service:
			kc.services[k8s.NamespacedNameOf(obj)] = obj
			return kc.serviceTriggersRebuild(obj), len(kc.services)

		case *core_v1.Namespace:
			kc.namespaces[obj.Name] = obj
			return true, len(kc.namespaces)

		case *networking_v1.Ingress:
			if !ingressclass.MatchesIngress(obj, kc.IngressClassNames) {
				// We didn't get a match so report this object is being ignored.
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", k8s.KindOf(obj)).
					WithField("ingress-class-annotation", annotation.IngressClass(obj)).
					WithField("ingress-class-name", ptr.Deref(obj.Spec.IngressClassName, "")).
					WithField("target-ingress-classes", kc.IngressClassNames).
					Debug("ignoring Ingress with unmatched ingress class")
				return false, len(kc.ingresses)
			}
			kc.ingresses[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.ingresses)

		case *contour_v1.HTTPProxy:
			if !ingressclass.MatchesHTTPProxy(obj, kc.IngressClassNames) {
				// We didn't get a match so report this object is being ignored.
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", k8s.KindOf(obj)).
					WithField("ingress-class-annotation", annotation.IngressClass(obj)).
					WithField("ingress-class-name", obj.Spec.IngressClassName).
					WithField("target-ingress-classes", kc.IngressClassNames).
					Debug("ignoring HTTPProxy with unmatched ingress class")
				return false, len(kc.httpproxies)
			}

			kc.httpproxies[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.httpproxies)

		case *contour_v1.TLSCertificateDelegation:
			kc.tlscertificatedelegations[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.tlscertificatedelegations)

		case *gatewayapi_v1.GatewayClass:
			switch {
			// Specific gateway configured: make sure the incoming gateway class
			// matches that gateway's.
			case kc.ConfiguredGatewayToCache != nil:
				if kc.gateway == nil || obj.Name != string(kc.gateway.Spec.GatewayClassName) {
					if kc.gatewayclass == nil {
						return false, 0
					}
					return false, 1
				}

				kc.gatewayclass = obj
				return true, 1
			// Otherwise, take whatever we're given.
			default:
				kc.gatewayclass = obj
				return true, 1
			}

		case *gatewayapi_v1.Gateway:
			switch {
			// Specific gateway configured: make sure the incoming gateway
			// matches, and get its gateway class.
			case kc.ConfiguredGatewayToCache != nil:
				if k8s.NamespacedNameOf(obj) != *kc.ConfiguredGatewayToCache {
					if kc.gateway == nil {
						return false, 0
					}
					return false, 1
				}

				kc.gateway = obj

				gatewayClass := &gatewayapi_v1.GatewayClass{}
				if err := kc.Client.Get(context.Background(), client.ObjectKey{Name: string(kc.gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
					kc.WithError(err).Errorf("error getting gatewayclass for gateway %s/%s", kc.gateway.Namespace, kc.gateway.Name)
				} else {
					kc.gatewayclass = gatewayClass
				}

				return true, 1
			// Otherwise, take whatever we're given.
			default:
				kc.gateway = obj
				return true, 1
			}

		case *gatewayapi_v1.HTTPRoute:
			kc.httproutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.httproutes)

		case *gatewayapi_v1alpha2.TLSRoute:
			kc.tlsroutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.tlsroutes)

		case *gatewayapi_v1.GRPCRoute:
			kc.grpcroutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.grpcroutes)

		case *gatewayapi_v1alpha2.TCPRoute:
			kc.tcproutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.tcproutes)

		case *gatewayapi_v1beta1.ReferenceGrant:
			kc.referencegrants[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.referencegrants)

		case *gatewayapi_v1alpha3.BackendTLSPolicy:
			kc.backendtlspolicies[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.backendtlspolicies)

		case *contour_v1alpha1.ExtensionService:
			kc.extensions[k8s.NamespacedNameOf(obj)] = obj
			return true, len(kc.extensions)

		default:
			// not an interesting object
			kc.WithField("object", obj).Error("insert unknown object")
			return false, 0
		}
	}

	ok, count := maybeInsert(obj)
	kind := k8s.KindOf(obj)
	kc.Metrics.SetDAGCacheObjectMetric(kind, count)
	if ok {
		// Only check annotations if we actually inserted
		// the object in our cache; uninteresting objects
		// should not be checked.
		if obj, ok := obj.(meta_v1.Object); ok {
			for key := range obj.GetAnnotations() {
				// Emit a warning if this is a known annotation that has
				// been applied to an invalid object kind. Note that we
				// only warn for known annotations because we want to
				// allow users to add arbitrary orthogonal annotations
				// to objects that we inspect.
				if annotation.IsKnown(key) && !annotation.ValidForKind(kind, key) {
					// TODO(jpeach): this should be exposed
					// to the user as a status condition.
					kc.WithField("name", obj.GetName()).
						WithField("namespace", obj.GetNamespace()).
						WithField("kind", kind).
						WithField("version", k8s.VersionOf(obj)).
						WithField("annotation", key).
						Error("ignoring invalid or unsupported annotation")
				}
			}
		}

		return true
	}

	return false
}

// Remove removes obj from the KubernetesCache.
// Remove returns a boolean indicating if the cache changed after the remove operation.
func (kc *KubernetesCache) Remove(obj any) bool {
	kc.initialize.Do(kc.init)

	switch obj := obj.(type) {
	default:
		ok, count := kc.remove(obj)
		kc.Metrics.SetDAGCacheObjectMetric(k8s.KindOf(obj), count)
		return ok

	case cache.DeletedFinalStateUnknown:
		return kc.Remove(obj.Obj) // recurse into ourselves with the tombstoned value
	}
}

func (kc *KubernetesCache) remove(obj any) (bool, int) {
	switch obj := obj.(type) {
	case *core_v1.Secret:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.secrets, m)
		return kc.secretTriggersRebuild(obj), len(kc.secrets)

	case *core_v1.ConfigMap:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.configmapsecrets, m)
		return kc.configMapTriggersRebuild(obj), len(kc.configmapsecrets)

	case *core_v1.Service:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.services, m)
		return kc.serviceTriggersRebuild(obj), len(kc.services)

	case *core_v1.Namespace:
		_, ok := kc.namespaces[obj.Name]
		delete(kc.namespaces, obj.Name)
		return ok, len(kc.namespaces)

	case *networking_v1.Ingress:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.ingresses[m]
		delete(kc.ingresses, m)
		return ok, len(kc.ingresses)

	case *contour_v1.HTTPProxy:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.httpproxies[m]
		delete(kc.httpproxies, m)
		return ok, len(kc.httpproxies)

	case *contour_v1.TLSCertificateDelegation:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.tlscertificatedelegations[m]
		delete(kc.tlscertificatedelegations, m)
		return ok, len(kc.tlscertificatedelegations)

	case *gatewayapi_v1.GatewayClass:
		switch {
		case kc.ConfiguredGatewayToCache != nil:
			if kc.gatewayclass == nil {
				return false, 0
			}
			if obj.Name != kc.gatewayclass.Name {
				return false, 1
			}
			kc.gatewayclass = nil
			return true, 0

		default:
			kc.gatewayclass = nil
			return true, 0
		}

	case *gatewayapi_v1.Gateway:
		switch {
		case kc.ConfiguredGatewayToCache != nil:
			if kc.gateway == nil {
				return false, 0
			}
			if k8s.NamespacedNameOf(obj) != k8s.NamespacedNameOf(kc.gateway) {
				return false, 1
			}
			kc.gateway = nil
			return true, 0
		default:
			kc.gateway = nil
			return true, 0
		}
	case *gatewayapi_v1.HTTPRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.httproutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.httproutes)

	case *gatewayapi_v1alpha2.TLSRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.tlsroutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.tlsroutes)

	case *gatewayapi_v1.GRPCRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.grpcroutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.grpcroutes)

	case *gatewayapi_v1alpha2.TCPRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.tcproutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs), len(kc.tcproutes)

	case *gatewayapi_v1beta1.ReferenceGrant:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.referencegrants[m]
		delete(kc.referencegrants, m)
		return ok, len(kc.referencegrants)

	case *gatewayapi_v1alpha3.BackendTLSPolicy:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.backendtlspolicies[m]
		delete(kc.backendtlspolicies, m)
		return ok, len(kc.backendtlspolicies)

	case *contour_v1alpha1.ExtensionService:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.extensions[m]
		delete(kc.extensions, m)
		return ok, len(kc.extensions)

	default:
		// not interesting
		kc.WithField("object", obj).Error("remove unknown object")
		return false, 0
	}
}

// serviceTriggersRebuild returns true if this service is referenced
// by an Ingress or HTTPProxy in this cache.
func (kc *KubernetesCache) serviceTriggersRebuild(service *core_v1.Service) bool {
	for _, ingress := range kc.ingresses {
		if ingress.Namespace != service.Namespace {
			continue
		}
		if backend := ingress.Spec.DefaultBackend; backend != nil {
			if backend.Service.Name == service.Name {
				return true
			}
		}

		for _, rule := range ingress.Spec.Rules {
			http := rule.IngressRuleValue.HTTP
			if http == nil {
				continue
			}
			for _, path := range http.Paths {
				if path.Backend.Service.Name == service.Name {
					return true
				}
			}
		}
	}

	for _, proxy := range kc.httpproxies {
		if proxy.Namespace != service.Namespace {
			continue
		}
		for _, route := range proxy.Spec.Routes {
			for _, s := range route.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
		if tcpproxy := proxy.Spec.TCPProxy; tcpproxy != nil {
			for _, s := range tcpproxy.Services {
				if s.Name == service.Name {
					return true
				}
			}
		}
	}

	for _, route := range kc.grpcroutes {
		for _, rule := range route.Spec.Rules {
			for _, backend := range rule.BackendRefs {
				if isRefToService(backend.BackendObjectReference, service, route.Namespace) {
					return true
				}
			}
		}
	}

	for _, route := range kc.httproutes {
		for _, rule := range route.Spec.Rules {
			for _, backend := range rule.BackendRefs {
				if isRefToService(backend.BackendObjectReference, service, route.Namespace) {
					return true
				}
			}
		}
	}

	for _, route := range kc.tlsroutes {
		for _, rule := range route.Spec.Rules {
			for _, backend := range rule.BackendRefs {
				if isRefToService(backend.BackendObjectReference, service, route.Namespace) {
					return true
				}
			}
		}
	}

	for _, route := range kc.tcproutes {
		for _, rule := range route.Spec.Rules {
			for _, backend := range rule.BackendRefs {
				if isRefToService(backend.BackendObjectReference, service, route.Namespace) {
					return true
				}
			}
		}
	}

	return false
}

func isRefToService(ref gatewayapi_v1.BackendObjectReference, service *core_v1.Service, routeNamespace string) bool {
	return ref.Group != nil && *ref.Group == "" &&
		ref.Kind != nil && *ref.Kind == "Service" &&
		((ref.Namespace != nil && string(*ref.Namespace) == service.Namespace) || (ref.Namespace == nil && routeNamespace == service.Namespace)) &&
		string(ref.Name) == service.Name
}

// secretTriggersRebuild returns true if this secret is referenced by an Ingress
// or HTTPProxy object, or by the configuration file.
// If the secret is not in the same namespace the function ignores TLSCertificateDelegation.
// As a result, it may trigger rebuild even if the reference is invalid, which should be rare and not worth the added complexity.
// Permission is checked when the secret is actually accessed.
func (kc *KubernetesCache) secretTriggersRebuild(secretObj *core_v1.Secret) bool {
	if _, isCA := secretObj.Data[CACertificateKey]; isCA {
		// locating a secret validation usage involves traversing each
		// proxy object, determining if there is a valid delegation,
		// and if the reference the secret as a certificate. The DAG already
		// does this so don't reproduce the logic and just assume for the moment
		// that any change to a CA secret will trigger a rebuild.
		return true
	}

	secret := types.NamespacedName{
		Namespace: secretObj.Namespace,
		Name:      secretObj.Name,
	}

	for _, ingress := range kc.ingresses {
		for _, tls := range ingress.Spec.TLS {
			if secret == k8s.NamespacedNameFrom(tls.SecretName, k8s.TLSCertAnnotationNamespace(ingress), k8s.DefaultNamespace(ingress.Namespace)) {
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

		if secret == k8s.NamespacedNameFrom(tls.SecretName, k8s.DefaultNamespace(proxy.Namespace)) {
			return true
		}

		cv := tls.ClientValidation
		if cv != nil && secret == k8s.NamespacedNameFrom(cv.CertificateRevocationList, k8s.DefaultNamespace(proxy.Namespace)) {
			return true
		}
	}

	// Secrets referred by the configuration file shall also trigger rebuild.
	for _, s := range kc.ConfiguredSecretRefs {
		if secret == *s {
			return true
		}
	}

	if kc.gateway != nil {
		for _, listener := range kc.gateway.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}

			for _, certificateRef := range listener.TLS.CertificateRefs {
				if isRefToSecret(certificateRef, secretObj, kc.gateway.Namespace) {
					return true
				}
			}
		}
	}

	return false
}

func isRefToSecret(ref gatewayapi_v1.SecretObjectReference, secret *core_v1.Secret, gatewayNamespace string) bool {
	return ref.Group != nil && *ref.Group == "" &&
		ref.Kind != nil && *ref.Kind == "Secret" &&
		((ref.Namespace != nil && *ref.Namespace == gatewayapi_v1.Namespace(secret.Namespace)) || (ref.Namespace == nil && gatewayNamespace == secret.Namespace)) &&
		string(ref.Name) == secret.Name
}

// configMapTriggersRebuild returns true if this configmap is referenced by a
// BackendTLSPolicy object.
func (kc *KubernetesCache) configMapTriggersRebuild(configMapObj *core_v1.ConfigMap) bool {
	configMap := types.NamespacedName{
		Namespace: configMapObj.Namespace,
		Name:      configMapObj.Name,
	}

	for _, backendtlspolicy := range kc.backendtlspolicies {
		for _, caCertRef := range backendtlspolicy.Spec.Validation.CACertificateRefs {
			if caCertRef.Group != "" || caCertRef.Kind != "ConfigMap" {
				continue
			}

			caCertRefNamespacedName := types.NamespacedName{
				Namespace: backendtlspolicy.Namespace,
				Name:      string(caCertRef.Name),
			}
			if configMap == caCertRefNamespacedName {
				return true
			}
		}
	}
	return false
}

// routeTriggersRebuild returns true if this route references gateway in this cache.
func (kc *KubernetesCache) routeTriggersRebuild(parentRefs []gatewayapi_v1.ParentReference) bool {
	if kc.gateway == nil {
		return false
	}

	for _, parentRef := range parentRefs {
		if gatewayapi.IsRefToGateway(parentRef, k8s.NamespacedNameOf(kc.gateway)) {
			return true
		}
	}

	return false
}

// LookupTLSSecret returns Secret with TLS certificate and private key from cache.
// If name (referred Secret) is in different namespace than targetNamespace (the referring object),
// then delegation check is performed.
func (kc *KubernetesCache) LookupTLSSecret(name types.NamespacedName, targetNamespace string) (*Secret, error) {
	if !kc.delegationPermitted(name, targetNamespace) {
		return nil, NewDelegationNotPermittedError(fmt.Errorf("Certificate delegation not permitted"))
	}
	return kc.LookupTLSSecretInsecure(name)
}

// LookupCASecret returns Secret with CA certificate from cache.
// If name (referred Secret) is in different namespace than targetNamespace (the referring object),
// then delegation check is performed.
func (kc *KubernetesCache) LookupCASecret(name types.NamespacedName, targetNamespace string) (*Secret, error) {
	if !kc.delegationPermitted(name, targetNamespace) {
		return nil, NewDelegationNotPermittedError(fmt.Errorf("Certificate delegation not permitted"))
	}

	sec, ok := kc.secrets[name]
	if !ok {
		return nil, fmt.Errorf("Secret not found")
	}

	// Compute and store the validation result if not
	// already stored.
	if sec.ValidCASecret == nil {
		sec.ValidCASecret = &SecretValidationStatus{
			Error: validCASecret(sec.Object),
		}
	}

	if err := sec.ValidCASecret.Error; err != nil {
		return nil, err
	}
	return sec, nil
}

// LookupCAConfigMap returns ConfigMap converted into dag.Secret with CA certificate from cache.
func (kc *KubernetesCache) LookupCAConfigMap(name types.NamespacedName) (*Secret, error) {
	sec, ok := kc.configmapsecrets[name]
	if !ok {
		return nil, fmt.Errorf("ConfigMap not found")
	}

	// Compute and store the validation result if not
	// already stored.
	if sec.ValidCASecret == nil {
		sec.ValidCASecret = &SecretValidationStatus{
			Error: validCASecret(sec.Object),
		}
	}

	if err := sec.ValidCASecret.Error; err != nil {
		return nil, err
	}
	return sec, nil
}

// LookupCRLSecret returns Secret with CRL from the cache.
// If name (referred Secret) is in different namespace than targetNamespace (the referring object),
// then delegation check is performed.
func (kc *KubernetesCache) LookupCRLSecret(name types.NamespacedName, targetNamespace string) (*Secret, error) {
	if !kc.delegationPermitted(name, targetNamespace) {
		return nil, NewDelegationNotPermittedError(fmt.Errorf("Certificate delegation not permitted"))
	}

	sec, ok := kc.secrets[name]
	if !ok {
		return nil, fmt.Errorf("Secret not found")
	}

	// Compute and store the validation result if not
	// already stored.
	if sec.ValidCRLSecret == nil {
		sec.ValidCRLSecret = &SecretValidationStatus{
			Error: validCRLSecret(sec.Object),
		}
	}

	if err := sec.ValidCRLSecret.Error; err != nil {
		return nil, err
	}
	return sec, nil
}

// LookupUpstreamValidation constructs PeerValidationContext with CA certificate from the cache.
// If name (referred Secret) is in different namespace than targetNamespace (the referring object),
// then delegation check is performed.
func (kc *KubernetesCache) LookupUpstreamValidation(uv *contour_v1.UpstreamValidation, caCertificate types.NamespacedName, targetNamespace string) (*PeerValidationContext, error) {
	if uv == nil {
		// no upstream validation requested, nothing to do
		return nil, nil
	}

	pvc := &PeerValidationContext{}

	cacert, err := kc.LookupCASecret(caCertificate, targetNamespace)
	if err != nil {
		if _, ok := err.(DelegationNotPermittedError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("invalid CA Secret %q: %s", caCertificate, err)
	}
	pvc.CACertificates = []*Secret{
		cacert,
	}

	// CEL validation should enforce that SubjectName must be set if SubjectNames is used. So, SubjectName will always be present.
	if uv.SubjectName == "" {
		return nil, errors.New("missing subject alternative name")
	}

	switch l := len(uv.SubjectNames); {
	case l == 0:
		// UpstreamValidation was using old SubjectName field only, can internally move that into SubjectNames
		pvc.SubjectNames = []string{uv.SubjectName}
	case l > 0:
		// UpstreamValidation is using new SubjectNames field, can use it directly. CEL validation should enforce that SubjectName is contained in SubjectNames
		if uv.SubjectName != uv.SubjectNames[0] {
			return nil, fmt.Errorf("first entry of SubjectNames (%s) does not match SubjectName (%s)", uv.SubjectNames[0], uv.SubjectName)
		}
		pvc.SubjectNames = uv.SubjectNames
	}

	return pvc, nil
}

// LookupTLSSecretInsecure returns Secret with TLS certificate and private key from cache.
// No delegation check is performed.
func (kc *KubernetesCache) LookupTLSSecretInsecure(name types.NamespacedName) (*Secret, error) {
	sec, ok := kc.secrets[name]
	if !ok {
		return nil, fmt.Errorf("Secret not found")
	}

	// Compute and store the validation result if not
	// already stored.
	if sec.ValidTLSSecret == nil {
		sec.ValidTLSSecret = &SecretValidationStatus{
			Error: validTLSSecret(sec.Object),
		}
	}

	if err := sec.ValidTLSSecret.Error; err != nil {
		return nil, err
	}
	return sec, nil
}

// delegationPermitted returns true if the referenced secret has been delegated
// to the namespace where the ingress object is located.
func (kc *KubernetesCache) delegationPermitted(secret types.NamespacedName, targetNamespace string) bool {
	contains := func(haystack []string, needle string) bool {
		if len(haystack) == 1 && haystack[0] == "*" {
			return true
		}
		for _, h := range haystack {
			if h == needle {
				return true
			}
		}
		return false
	}

	if secret.Namespace == targetNamespace {
		// secret is in the same namespace as target
		return true
	}

	for _, d := range kc.tlscertificatedelegations {
		if d.Namespace != secret.Namespace {
			continue
		}
		for _, d := range d.Spec.Delegations {
			if contains(d.TargetNamespaces, targetNamespace) {
				if secret.Name == d.SecretName {
					return true
				}
			}
		}
	}
	return false
}

// LookupService returns the Kubernetes service and port matching the provided parameters,
// or an error if a match can't be found.
func (kc *KubernetesCache) LookupService(meta types.NamespacedName, port intstr.IntOrString) (*core_v1.Service, core_v1.ServicePort, error) {
	svc, ok := kc.services[meta]
	if !ok {
		return nil, core_v1.ServicePort{}, fmt.Errorf("service %q not found", meta)
	}

	for i := range svc.Spec.Ports {
		p := svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() || port.String() == p.Name {
			switch p.Protocol {
			case "", core_v1.ProtocolTCP:
				return svc, p, nil
			default:
				return nil, core_v1.ServicePort{}, fmt.Errorf("unsupported service protocol %q", p.Protocol)
			}
		}
	}

	return nil, core_v1.ServicePort{}, fmt.Errorf("port %q on service %q not matched", port.String(), meta)
}

// LookupBackendTLSPolicyByTargetRef returns the Kubernetes BackendTLSPolicy that matches the provided targetRef with
// a SectionName, if possible. A BackendTLSPolicy may be returned if there is a BackendTLSPolicy matching the targetRef
// but has no SectionName.
//
// For example, there could be two BackendTLSPolicies matching Service "foo". One of them matches SectionName "https",
// but the other has no SectionName and functions as a catch-all policy for service "foo".
//
// The namespace provided is intended to be the namespace of the backend we are looking up a reference to (since only
// namespace-local references are allowed) and is used to match the namespace on the resulting backendTLSPolicy.
//
// If a policy is found, true is returned.
func (kc *KubernetesCache) LookupBackendTLSPolicyByTargetRef(targetRef gatewayapi_v1alpha2.LocalPolicyTargetReferenceWithSectionName, namespace string) (*gatewayapi_v1alpha3.BackendTLSPolicy, bool) {
	var fallbackBackendTLSPolicy *gatewayapi_v1alpha3.BackendTLSPolicy
	for _, v := range kc.backendtlspolicies {
		// Make sure the BackendTLSPolicy namespace matches the backend namespace.
		if v.Namespace != namespace {
			continue
		}

		// One of the Policy target refs must match the expected target ref.
		for _, tr := range v.Spec.TargetRefs {
			sectionNameMatches := tr.SectionName != nil && targetRef.SectionName != nil &&
				*tr.SectionName == *targetRef.SectionName

			if tr.LocalPolicyTargetReference.Group == targetRef.Group &&
				tr.LocalPolicyTargetReference.Kind == targetRef.Kind &&
				tr.LocalPolicyTargetReference.Name == targetRef.Name {
				if sectionNameMatches {
					return v, true
				}

				if tr.SectionName == nil {
					fallbackBackendTLSPolicy = v
				}
			}
		}
	}

	if fallbackBackendTLSPolicy != nil {
		return fallbackBackendTLSPolicy, true
	}

	return nil, false
}

func (kc *KubernetesCache) convertCACertConfigMapToSecret(configMap *core_v1.ConfigMap) (*core_v1.Secret, bool) {
	if _, ok := configMap.Data[CACertificateKey]; !ok {
		return nil, false
	}

	return &core_v1.Secret{
		ObjectMeta: configMap.ObjectMeta,
		Data: map[string][]byte{
			CACertificateKey: []byte(configMap.Data[CACertificateKey]),
		},
		Type: core_v1.SecretTypeOpaque,
	}, true
}
