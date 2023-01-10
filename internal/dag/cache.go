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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/ingressclass"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
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
	httpproxies               map[types.NamespacedName]*contour_api_v1.HTTPProxy
	secrets                   map[types.NamespacedName]*Secret
	tlscertificatedelegations map[types.NamespacedName]*contour_api_v1.TLSCertificateDelegation
	services                  map[types.NamespacedName]*v1.Service
	namespaces                map[string]*v1.Namespace
	gatewayclass              *gatewayapi_v1beta1.GatewayClass
	gateway                   *gatewayapi_v1beta1.Gateway
	httproutes                map[types.NamespacedName]*gatewayapi_v1beta1.HTTPRoute
	tlsroutes                 map[types.NamespacedName]*gatewayapi_v1alpha2.TLSRoute
	referencegrants           map[types.NamespacedName]*gatewayapi_v1beta1.ReferenceGrant
	extensions                map[types.NamespacedName]*contour_api_v1alpha1.ExtensionService

	Client client.Reader

	initialize sync.Once

	logrus.FieldLogger
}

// init creates the internal cache storage. It is called implicitly from the public API.
func (kc *KubernetesCache) init() {
	kc.ingresses = make(map[types.NamespacedName]*networking_v1.Ingress)
	kc.httpproxies = make(map[types.NamespacedName]*contour_api_v1.HTTPProxy)
	kc.secrets = make(map[types.NamespacedName]*Secret)
	kc.tlscertificatedelegations = make(map[types.NamespacedName]*contour_api_v1.TLSCertificateDelegation)
	kc.services = make(map[types.NamespacedName]*v1.Service)
	kc.namespaces = make(map[string]*v1.Namespace)
	kc.httproutes = make(map[types.NamespacedName]*gatewayapi_v1beta1.HTTPRoute)
	kc.referencegrants = make(map[types.NamespacedName]*gatewayapi_v1beta1.ReferenceGrant)
	kc.tlsroutes = make(map[types.NamespacedName]*gatewayapi_v1alpha2.TLSRoute)
	kc.extensions = make(map[types.NamespacedName]*contour_api_v1alpha1.ExtensionService)
}

// Insert inserts obj into the KubernetesCache.
// Insert returns true if the cache accepted the object, or false if the value
// is not interesting to the cache. If an object with a matching type, name,
// and namespace exists, it will be overwritten.
func (kc *KubernetesCache) Insert(obj interface{}) bool {
	kc.initialize.Do(kc.init)

	maybeInsert := func(obj interface{}) bool {
		switch obj := obj.(type) {
		case *v1.Secret:
			// Secret validation status is intentionally cleared, it needs
			// to be re-validated after an insert.
			kc.secrets[k8s.NamespacedNameOf(obj)] = &Secret{Object: obj}
			return kc.secretTriggersRebuild(obj)
		case *v1.Service:
			kc.services[k8s.NamespacedNameOf(obj)] = obj
			return kc.serviceTriggersRebuild(obj)
		case *v1.Namespace:
			kc.namespaces[obj.Name] = obj
			return true
		case *networking_v1.Ingress:
			if !ingressclass.MatchesIngress(obj, kc.IngressClassNames) {
				// We didn't get a match so report this object is being ignored.
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", k8s.KindOf(obj)).
					WithField("ingress-class-annotation", annotation.IngressClass(obj)).
					WithField("ingress-class-name", ref.Val(obj.Spec.IngressClassName, "")).
					WithField("target-ingress-classes", kc.IngressClassNames).
					Debug("ignoring Ingress with unmatched ingress class")
				return false
			}

			kc.ingresses[k8s.NamespacedNameOf(obj)] = obj
			return true
		case *contour_api_v1.HTTPProxy:
			if !ingressclass.MatchesHTTPProxy(obj, kc.IngressClassNames) {
				// We didn't get a match so report this object is being ignored.
				kc.WithField("name", obj.GetName()).
					WithField("namespace", obj.GetNamespace()).
					WithField("kind", k8s.KindOf(obj)).
					WithField("ingress-class-annotation", annotation.IngressClass(obj)).
					WithField("ingress-class-name", obj.Spec.IngressClassName).
					WithField("target-ingress-classes", kc.IngressClassNames).
					Debug("ignoring HTTPProxy with unmatched ingress class")
				return false
			}

			kc.httpproxies[k8s.NamespacedNameOf(obj)] = obj
			return true
		case *contour_api_v1.TLSCertificateDelegation:
			kc.tlscertificatedelegations[k8s.NamespacedNameOf(obj)] = obj
			return true
		case *gatewayapi_v1beta1.GatewayClass:
			switch {
			// Specific gateway configured: make sure the incoming gateway class
			// matches that gateway's.
			case kc.ConfiguredGatewayToCache != nil:
				if kc.gateway == nil || obj.Name != string(kc.gateway.Spec.GatewayClassName) {
					return false
				}

				kc.gatewayclass = obj
				return true
			// Otherwise, take whatever we're given.
			default:
				kc.gatewayclass = obj
				return true
			}
		case *gatewayapi_v1beta1.Gateway:
			switch {
			// Specific gateway configured: make sure the incoming gateway
			// matches, and get its gateway class.
			case kc.ConfiguredGatewayToCache != nil:
				if k8s.NamespacedNameOf(obj) != *kc.ConfiguredGatewayToCache {
					return false
				}

				kc.gateway = obj

				gatewayClass := &gatewayapi_v1beta1.GatewayClass{}
				if err := kc.Client.Get(context.Background(), client.ObjectKey{Name: string(kc.gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
					kc.WithError(err).Errorf("error getting gatewayclass for gateway %s/%s", kc.gateway.Namespace, kc.gateway.Name)
				} else {
					kc.gatewayclass = gatewayClass
				}

				return true
			// Otherwise, take whatever we're given.
			default:
				kc.gateway = obj
				return true
			}
		case *gatewayapi_v1beta1.HTTPRoute:
			kc.httproutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs)
		case *gatewayapi_v1alpha2.TLSRoute:
			kc.tlsroutes[k8s.NamespacedNameOf(obj)] = obj
			return kc.routeTriggersRebuild(obj.Spec.ParentRefs)
		case *gatewayapi_v1beta1.ReferenceGrant:
			kc.referencegrants[k8s.NamespacedNameOf(obj)] = obj
			return true
		case *contour_api_v1alpha1.ExtensionService:
			kc.extensions[k8s.NamespacedNameOf(obj)] = obj
			return true
		case *contour_api_v1alpha1.ContourConfiguration:
			return false
		default:
			// not an interesting object
			kc.WithField("object", obj).Error("insert unknown object")
			return false
		}
	}

	if maybeInsert(obj) {
		// Only check annotations if we actually inserted
		// the object in our cache; uninteresting objects
		// should not be checked.
		if obj, ok := obj.(metav1.Object); ok {
			kind := k8s.KindOf(obj)
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
func (kc *KubernetesCache) Remove(obj interface{}) bool {
	kc.initialize.Do(kc.init)

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
		m := k8s.NamespacedNameOf(obj)
		delete(kc.secrets, m)
		return kc.secretTriggersRebuild(obj)
	case *v1.Service:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.services, m)
		return kc.serviceTriggersRebuild(obj)
	case *v1.Namespace:
		_, ok := kc.namespaces[obj.Name]
		delete(kc.namespaces, obj.Name)
		return ok
	case *networking_v1.Ingress:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.ingresses[m]
		delete(kc.ingresses, m)
		return ok
	case *contour_api_v1.HTTPProxy:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.httpproxies[m]
		delete(kc.httpproxies, m)
		return ok
	case *contour_api_v1.TLSCertificateDelegation:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.tlscertificatedelegations[m]
		delete(kc.tlscertificatedelegations, m)
		return ok
	case *gatewayapi_v1beta1.GatewayClass:
		switch {
		case kc.ConfiguredGatewayToCache != nil:
			if kc.gatewayclass == nil || obj.Name != kc.gatewayclass.Name {
				return false
			}
			kc.gatewayclass = nil
			return true
		default:
			kc.gatewayclass = nil
			return true
		}
	case *gatewayapi_v1beta1.Gateway:
		switch {
		case kc.ConfiguredGatewayToCache != nil:
			if kc.gateway == nil || k8s.NamespacedNameOf(obj) != k8s.NamespacedNameOf(kc.gateway) {
				return false
			}
			kc.gateway = nil
			return true
		default:
			kc.gateway = nil
			return true
		}
	case *gatewayapi_v1beta1.HTTPRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.httproutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs)
	case *gatewayapi_v1alpha2.TLSRoute:
		m := k8s.NamespacedNameOf(obj)
		delete(kc.tlsroutes, m)
		return kc.routeTriggersRebuild(obj.Spec.ParentRefs)
	case *gatewayapi_v1beta1.ReferenceGrant:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.referencegrants[m]
		delete(kc.referencegrants, m)
		return ok
	case *contour_api_v1alpha1.ExtensionService:
		m := k8s.NamespacedNameOf(obj)
		_, ok := kc.extensions[m]
		delete(kc.extensions, m)
		return ok
	case *contour_api_v1alpha1.ContourConfiguration:
		return false
	default:
		// not interesting
		kc.WithField("object", obj).Error("remove unknown object")
		return false
	}
}

// serviceTriggersRebuild returns true if this service is referenced
// by an Ingress or HTTPProxy in this cache.
func (kc *KubernetesCache) serviceTriggersRebuild(service *v1.Service) bool {
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

	return false
}

func isRefToService(ref gatewayapi_v1beta1.BackendObjectReference, service *v1.Service, routeNamespace string) bool {
	return ref.Group != nil && *ref.Group == "" &&
		ref.Kind != nil && *ref.Kind == "Service" &&
		((ref.Namespace != nil && string(*ref.Namespace) == service.Namespace) || (ref.Namespace == nil && routeNamespace == service.Namespace)) &&
		string(ref.Name) == service.Name
}

// secretTriggersRebuild returns true if this secret is referenced by an Ingress
// or HTTPProxy object, or by the configuration file. If the secret is not in the same namespace
// it must be mentioned by a TLSCertificateDelegation.
func (kc *KubernetesCache) secretTriggersRebuild(secret *v1.Secret) bool {
	if _, isCA := secret.Data[CACertificateKey]; isCA {
		// locating a secret validation usage involves traversing each
		// proxy object, determining if there is a valid delegation,
		// and if the reference the secret as a certificate. The DAG already
		// does this so don't reproduce the logic and just assume for the moment
		// that any change to a CA secret will trigger a rebuild.
		return true
	}

	delegations := make(map[string]bool) // targetnamespace/secretname to bool

	// TODO(youngnick): Check if this is required.
	for _, d := range kc.tlscertificatedelegations {
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

	// Secrets referred by the configuration file shall also trigger rebuild.
	for _, s := range kc.ConfiguredSecretRefs {
		if s.Namespace == secret.Namespace && s.Name == secret.Name {
			return true
		}
	}

	if kc.gateway != nil {
		for _, listener := range kc.gateway.Spec.Listeners {
			if listener.TLS == nil {
				continue
			}

			for _, certificateRef := range listener.TLS.CertificateRefs {
				if isRefToSecret(certificateRef, secret, kc.gateway.Namespace) {
					return true
				}
			}
		}
	}

	return false
}

func isRefToSecret(ref gatewayapi_v1beta1.SecretObjectReference, secret *v1.Secret, gatewayNamespace string) bool {
	return ref.Group != nil && *ref.Group == "" &&
		ref.Kind != nil && *ref.Kind == "Secret" &&
		((ref.Namespace != nil && *ref.Namespace == gatewayapi_v1beta1.Namespace(secret.Namespace)) || (ref.Namespace == nil && gatewayNamespace == secret.Namespace)) &&
		string(ref.Name) == secret.Name
}

// routesTriggersRebuild returns true if this route references gateway in this cache.
func (kc *KubernetesCache) routeTriggersRebuild(parentRefs []gatewayapi_v1beta1.ParentReference) bool {
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

func (kc *KubernetesCache) LookupTLSSecret(name types.NamespacedName) (*Secret, error) {
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

func (kc *KubernetesCache) LookupCASecret(name types.NamespacedName) (*Secret, error) {
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

func (kc *KubernetesCache) LookupCRLSecret(name types.NamespacedName) (*Secret, error) {
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

func (kc *KubernetesCache) LookupUpstreamValidation(uv *contour_api_v1.UpstreamValidation, caCertificate types.NamespacedName) (*PeerValidationContext, error) {
	if uv == nil {
		// no upstream validation requested, nothing to do
		return nil, nil
	}

	cacert, err := kc.LookupCASecret(caCertificate)
	if err != nil {
		// UpstreamValidation is requested, but cert is missing or not configured
		return nil, fmt.Errorf("invalid CA Secret %q: %s", caCertificate, err)
	}

	if uv.SubjectName == "" {
		// UpstreamValidation is requested, but SAN is not provided
		return nil, errors.New("missing subject alternative name")
	}

	return &PeerValidationContext{
		CACertificate: cacert,
		SubjectName:   uv.SubjectName,
	}, nil
}

// DelegationPermitted returns true if the referenced secret has been delegated
// to the namespace where the ingress object is located.
func (kc *KubernetesCache) DelegationPermitted(secret types.NamespacedName, targetNamespace string) bool {
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
func (kc *KubernetesCache) LookupService(meta types.NamespacedName, port intstr.IntOrString) (*v1.Service, v1.ServicePort, error) {
	svc, ok := kc.services[meta]
	if !ok {
		return nil, v1.ServicePort{}, fmt.Errorf("service %q not found", meta)
	}

	for i := range svc.Spec.Ports {
		p := svc.Spec.Ports[i]
		if int(p.Port) == port.IntValue() || port.String() == p.Name {
			switch p.Protocol {
			case "", v1.ProtocolTCP:
				return svc, p, nil
			default:
				return nil, v1.ServicePort{}, fmt.Errorf("unsupported service protocol %q", p.Protocol)
			}
		}
	}

	return nil, v1.ServicePort{}, fmt.Errorf("port %q on service %q not matched", port.String(), meta)
}
