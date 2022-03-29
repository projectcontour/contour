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

package model

import (
	"fmt"
	"strings"
)

// ConfigMapName returns the name of the Contour ConfigMap resource.
func (c *Contour) ConfigMapName() string {
	return "contour-" + c.Name
}

// ContourServiceName returns the name of the Contour Service resource.
func (c *Contour) ContourServiceName() string {
	return "contour-" + c.Name
}

// EnvoyServiceName returns the name of the Envoy Service resource.
func (c *Contour) EnvoyServiceName() string {
	return "envoy-" + c.Name
}

// ContourDeploymentName returns the name of the Contour Deployment resource.
func (c *Contour) ContourDeploymentName() string {
	return "contour-" + c.Name
}

// EnvoyDaemonSetName returns the name of the Envoy DaemonSet resource.
func (c *Contour) EnvoyDaemonSetName() string {
	return "envoy-" + c.Name
}

// LeaderElectionLeaseName returns the name of the Contour leader election Lease resource.
func (c *Contour) LeaderElectionLeaseName() string {
	return "leader-elect-" + c.Name
}

// ContourCertsSecretName returns the name of the Contour xDS TLS certs Secret resource.
func (c *Contour) ContourCertsSecretName() string {
	return c.Name + "-contourcert"
}

// EnvoyCertsSecretName returns the name of the Envoy xDS TLS certs Secret resource.
func (c *Contour) EnvoyCertsSecretName() string {
	return c.Name + "-envoycert"
}

// CertgenJobName returns the name of the certgen Job resource.
func (c *Contour) CertgenJobName(contourImage string) string {
	return fmt.Sprintf("contour-certgen-%s-%s", tagFromImage(contourImage), c.Name)
}

// tagFromImage returns the tag from the provided image or an
// empty string if the image does not contain a tag.
func tagFromImage(image string) string {
	if strings.Contains(image, ":") {
		parsed := strings.Split(image, ":")
		return parsed[1]
	}
	return ""
}

// ContourRBACNames returns the names of the RBAC resources for
// the Contour deployment.
func (c *Contour) ContourRBACNames() RBACNames {
	return RBACNames{
		ServiceAccount:     fmt.Sprintf("contour-%s", c.Name),
		ClusterRole:        fmt.Sprintf("contour-%s-%s", c.Namespace, c.Name),
		ClusterRoleBinding: fmt.Sprintf("contour-%s-%s", c.Namespace, c.Name),
		Role:               fmt.Sprintf("contour-%s", c.Name),

		// this one has a different prefix to differentiate from the certgen role binding (see below).
		RoleBinding: fmt.Sprintf("contour-rolebinding-%s", c.Name),
	}
}

// EnvoyRBACNames returns the names of the RBAC resources for
// the Envoy daemonset.
func (c *Contour) EnvoyRBACNames() RBACNames {
	return RBACNames{
		ServiceAccount: "envoy-" + c.Name,
	}
}

// CertgenRBACNames returns the names of the RBAC resources for
// the Certgen job.
func (c *Contour) CertgenRBACNames() RBACNames {
	return RBACNames{
		ServiceAccount: "contour-certgen-" + c.Name,
		Role:           "contour-certgen-" + c.Name,

		// this one is name contour-<gateway-name> despite being for certgen for legacy reasons.
		RoleBinding: "contour-" + c.Name,
	}
}

// RBACNames holds a set of names of related RBAC resources.
type RBACNames struct {
	ServiceAccount     string
	ClusterRole        string
	ClusterRoleBinding string
	Role               string
	RoleBinding        string
}
