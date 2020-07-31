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

	"github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ExtensionServiceProcessor struct {
	logrus.FieldLogger
}

var _ Processor = &ExtensionServiceProcessor{}

func (p *ExtensionServiceProcessor) Run(builder *Builder) {
	for n, e := range builder.Source.extensions {
		if ext := p.buildExtensionService(builder, e); ext != nil {
			builder.extensions[n] = ext
		}
	}
}

// buildExtensionService builds one ExtensionCluster record based
// on the corresponding CRD.
//
// TODO(jpeach): Publish status conditions in https://github.com/projectcontour/contour/issues/2874.
func (p *ExtensionServiceProcessor) buildExtensionService(
	builder *Builder,
	ext *v1alpha1.ExtensionService,
) *ExtensionCluster {
	// Generate a unique Envoy cluster name for an ExtensionCluster.
	// The namespaced name of an ExtensionCluster is globally
	// unique, so we can simply use that as the cluster name. As
	// long as we scope the context with the "extension" prefix
	// there can't be a conflict. Note that the name doesn't include
	// a hash of the contents because we want a 1-1 mapping between
	// ExtensionServices and Envoy Clusters; we don't want a new
	// Envoy Cluster just because a field changed.
	nameOf := func(ext *v1alpha1.ExtensionService) string {
		n := k8s.NamespacedNameOf(ext)
		return strings.Join([]string{"extension", n.Namespace, n.Name}, "/")
	}

	extension := ExtensionCluster{
		Name: nameOf(ext),
		Upstream: ServiceCluster{
			ClusterName: path.Join(
				"extension",
				xds.ClusterLoadAssignmentName(k8s.NamespacedNameOf(ext), ""),
			),
		},
		Protocol:           "h2",
		UpstreamValidation: nil,
		LoadBalancerPolicy: loadBalancerPolicy(ext.Spec.LoadBalancerPolicy),
		TimeoutPolicy:      timeoutPolicy(ext.Spec.TimeoutPolicy),
		SNI:                "",
	}

	// Timeouts are specified above the cluster (e.g.
	// in the ext_authz filter). The ext_authz filter
	// doesn't have an idle timeout (only a request
	// timeout), so validate that it is not provided here.
	if timeouts := ext.Spec.TimeoutPolicy; timeouts != nil && timeouts.Idle != "" {
		// TODO(jpeach): Add status condition, #2874.
		p.Infof("%s ignored", ".Spec.TimeoutPolicy.Idle")
	}

	// API server validation ensures that the protocol is "h2" or "h2c".
	if ext.Spec.Protocol != nil {
		extension.Protocol = stringOrDefault(*ext.Spec.Protocol, extension.Protocol)
	}

	if v := ext.Spec.UpstreamValidation; v != nil {
		uv, err := builder.Source.LookupUpstreamValidation(v, ext.GetNamespace())
		if err != nil {
			// TODO(jpeach): Add status condition, #2874.
			p.WithError(err).Error("failed to resolve upstream validation")
			return nil
		}

		if extension.Protocol != "h2" {
			// TODO(jpeach): Add status condition, #2874.
			p.WithError(err).Errorf(
				"upstream TLS validation not supported for %q protocol", extension.Protocol)
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
		extension.SNI = uv.SubjectName
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

		svc, err := builder.lookupService(svcName, intstr.FromInt(target.Port))
		if err != nil {
			// TODO(jpeach): Add status condition, #2874.
			p.WithError(err).
				WithField("name", svcName).
				WithField("port", target.Port).
				Error("failed to look up service")
			return nil
		}

		// TODO(jpeach): Add ExternalName support in https://github.com/projectcontour/contour/issues/2875.
		if svc.ExternalName != "" {
			p.WithError(err).
				WithField("name", svcName).
				WithField("port", target.Port).
				Error("failed to look up service")
			return nil
		}

		extension.Upstream.AddWeightedService(target.Weight, svcName, svc.Weighted.ServicePort)
	}

	return &extension
}
