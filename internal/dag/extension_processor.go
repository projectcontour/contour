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
	"path"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/status"
	"github.com/projectcontour/contour/internal/xds"
)

type ExtensionServiceProcessor struct {
	logrus.FieldLogger

	// ClientCertificate is the optional identifier of the TLS
	// secret containing client certificate and private key to be
	// used when establishing TLS connection to upstream cluster.
	ClientCertificate *types.NamespacedName

	// ConnectTimeout defines how long the proxy should wait when establishing connection to upstream service.
	ConnectTimeout time.Duration

	// UpstreamTLS defines the TLS settings like min/max version
	// and cipher suites for upstream connections.
	UpstreamTLS *UpstreamTLS

	// GlobalCircuitBreakerDefaults defines global circuit breaker defaults.
	GlobalCircuitBreakerDefaults *contour_v1alpha1.CircuitBreakers
}

var _ Processor = &ExtensionServiceProcessor{}

func (p *ExtensionServiceProcessor) Run(dag *DAG, cache *KubernetesCache) {
	for _, e := range cache.extensions {
		extStatus, commit := status.ExtensionAccessor(&dag.StatusCache, e)
		validCondition := extStatus.ConditionFor(status.ValidCondition)

		if ext := p.buildExtensionService(cache, e, validCondition); ext != nil {
			if len(validCondition.Errors) == 0 {
				dag.ExtensionClusters = append(dag.ExtensionClusters, ext)
			}
		}

		if len(validCondition.Errors) == 0 {
			validCondition.Status = contour_v1.ConditionTrue
			validCondition.Reason = "Valid"
			validCondition.Message = "Valid ExtensionService"
		}

		commit()
	}
}

// ExtensionClusterName generates a unique Envoy cluster name
// for an ExtensionCluster.
// The namespaced name of an ExtensionCluster is globally
// unique, so we can simply use that as the cluster name. As
// long as we scope the context with the "extension" prefix
// there can't be a conflict. Note that the name doesn't include
// a hash of the contents because we want a 1-1 mapping between
// ExtensionServices and Envoy Clusters; we don't want a new
// Envoy Cluster just because a field changed.
func ExtensionClusterName(meta types.NamespacedName) string {
	return strings.Join([]string{"extension", meta.Namespace, meta.Name}, "/")
}

// buildExtensionService builds one ExtensionCluster record based
// on the corresponding CRD.
func (p *ExtensionServiceProcessor) buildExtensionService(
	cache *KubernetesCache,
	ext *contour_v1alpha1.ExtensionService,
	validCondition *contour_v1.DetailedCondition,
) *ExtensionCluster {
	rtp, ctp, err := timeoutPolicy(ext.Spec.TimeoutPolicy, p.ConnectTimeout)
	if err != nil {
		validCondition.AddErrorf(contour_v1.ConditionTypeSpecError, "TimeoutPolicyNotValid",
			"spec.timeoutPolicy failed to parse: %s", err)
	}

	var clientCertSecret *Secret
	if p.ClientCertificate != nil {
		// Since the client certificate is configured by admin, explicit delegation is not required.
		clientCertSecret, err = cache.LookupTLSSecretInsecure(*p.ClientCertificate)
		if err != nil {
			validCondition.AddErrorf(contour_v1.ConditionTypeTLSError, "SecretNotValid",
				"tls.envoy-client-certificate Secret %q is invalid: %s", p.ClientCertificate, err)
		}
	}

	extension := ExtensionCluster{
		Name: ExtensionClusterName(k8s.NamespacedNameOf(ext)),
		Upstream: ServiceCluster{
			ClusterName: path.Join(
				"extension",
				xds.ClusterLoadAssignmentName(k8s.NamespacedNameOf(ext), ""),
			),
		},
		Protocol:             "h2",
		UpstreamValidation:   nil,
		RouteTimeoutPolicy:   rtp,
		ClusterTimeoutPolicy: ctp,
		SNI:                  "",
		ClientCertificate:    clientCertSecret,
		UpstreamTLS:          p.UpstreamTLS,
	}

	if p.GlobalCircuitBreakerDefaults != nil {
		extension.CircuitBreakers = CircuitBreakers{
			MaxConnections:        p.GlobalCircuitBreakerDefaults.MaxConnections,
			MaxPendingRequests:    p.GlobalCircuitBreakerDefaults.MaxPendingRequests,
			MaxRequests:           p.GlobalCircuitBreakerDefaults.MaxRequests,
			MaxRetries:            p.GlobalCircuitBreakerDefaults.MaxRetries,
			PerHostMaxConnections: p.GlobalCircuitBreakerDefaults.PerHostMaxConnections,
		}
	}

	if ext.Spec.CircuitBreakerPolicy != nil {
		extension.CircuitBreakers = CircuitBreakers{
			MaxConnections:        ext.Spec.CircuitBreakerPolicy.MaxConnections,
			MaxPendingRequests:    ext.Spec.CircuitBreakerPolicy.MaxPendingRequests,
			MaxRequests:           ext.Spec.CircuitBreakerPolicy.MaxRequests,
			MaxRetries:            ext.Spec.CircuitBreakerPolicy.MaxRetries,
			PerHostMaxConnections: ext.Spec.CircuitBreakerPolicy.PerHostMaxConnections,
		}
	}

	lbPolicy := loadBalancerPolicy(ext.Spec.LoadBalancerPolicy)
	switch lbPolicy {
	case LoadBalancerPolicyCookie, LoadBalancerPolicyRequestHash:
		validCondition.AddWarningf(contour_v1.ConditionTypeSpecError, "IgnoredField",
			"ignoring field %q; %s load balancer policy is not supported for ExtensionClusters",
			".Spec.LoadBalancerPolicy", lbPolicy)
		// Reset load balancer policy to ensure the default.
		lbPolicy = ""
	}
	extension.LoadBalancerPolicy = lbPolicy

	// Timeouts are specified above the cluster (e.g.
	// in the ext_authz filter). The ext_authz filter
	// doesn't have an idle timeout (only a request
	// timeout), so validate that it is not provided here.
	if timeouts := ext.Spec.TimeoutPolicy; timeouts != nil && timeouts.Idle != "" {
		validCondition.AddWarningf("SpecError", "IgnoredField",
			"ignoring field %q; idle timeouts are not supported for ExtensionClusters",
			".Spec.TimeoutPolicy.Idle")
	}

	// API server validation ensures that the protocol is "h2" or "h2c".
	if ext.Spec.Protocol != nil {
		extension.Protocol = stringOrDefault(*ext.Spec.Protocol, extension.Protocol)
	}

	if v := ext.Spec.UpstreamValidation; v != nil {
		// If the CACertificate name in the UpstreamValidation is namespaced and the namespace
		// is not the ExtensionService's namespace, check if the referenced secret is permitted to be
		// delegated to the ExtensionService's namespace.
		// By default, a non-namespaced CACertificate is expected to reside in the ExtensionService's namespace.
		caCertNamespacedName := k8s.NamespacedNameFrom(v.CACertificate, k8s.DefaultNamespace(ext.Namespace))
		uv, err := cache.LookupUpstreamValidation(v, caCertNamespacedName, ext.Namespace)
		if err != nil {
			if _, ok := err.(DelegationNotPermittedError); ok {
				validCondition.AddErrorf(contour_v1.ConditionTypeTLSError, "CACertificateNotDelegated",
					"service.UpstreamValidation.CACertificate Secret %q is not configured for certificate delegation", caCertNamespacedName)
			} else {
				validCondition.AddErrorf(contour_v1.ConditionTypeSpecError, "TLSUpstreamValidation",
					"TLS upstream validation policy error: %s", err.Error())
			}
			return nil
		}

		extension.UpstreamValidation = uv

		// Default the SNI server name to the name
		// we need to validate. It is a bit onerous
		// to also have to provide a CA bundle here,
		// but maybe we can make that optional in the
		// future.
		//
		// TODO(jpeach): expose SNI in the API, https://github.com/projectcontour/contour/issues/2893.
		extension.SNI = uv.SubjectNames[0]

		if extension.Protocol != "h2" {
			validCondition.AddErrorf(contour_v1.ConditionTypeSpecError, "InconsistentProtocol",
				"upstream TLS validation not supported for %q protocol", extension.Protocol)
		}
	}

	for _, target := range ext.Spec.Services {
		// Note that ExtensionServices only expose Kubernetes
		// Service resources that are in the same namespace.
		// This prevent using a cross-namespace reference to
		// subvert the Contour installation.
		svcName := types.NamespacedName{
			Namespace: ext.GetNamespace(),
			Name:      target.Name,
		}

		svc, port, err := cache.LookupService(svcName, intstr.FromInt(target.Port))
		if err != nil {
			validCondition.AddErrorf(contour_v1.ConditionTypeServiceError, "ServiceUnresolvedReference",
				"unresolved service %q: %s", svcName, err)
			continue
		}

		// TODO(jpeach): Add ExternalName support in https://github.com/projectcontour/contour/issues/2875.
		// TODO(youngnick): If ExternalName support is added, we must pass down the EnableExternalNameService bool
		// and check it first.
		if svc.Spec.ExternalName != "" {
			validCondition.AddErrorf(contour_v1.ConditionTypeServiceError, "UnsupportedServiceType",
				"Service %q is of unsupported type %q.", svcName, core_v1.ServiceTypeExternalName)
			continue
		}

		extension.Upstream.AddWeightedService(target.Weight, svcName, port)
	}

	return &extension
}
