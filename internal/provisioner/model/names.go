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
)

// ContourConfigurationName returns the name of the ContourConfiguration resource.
func (c *Contour) ContourConfigurationName() string {
	return "contourconfig-" + c.Name
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
	return "contourcert-" + c.Name
}

// EnvoyCertsSecretName returns the name of the Envoy xDS TLS certs Secret resource.
func (c *Contour) EnvoyCertsSecretName() string {
	return "envoycert-" + c.Name
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

// RBACNames holds a set of names of related RBAC resources.
type RBACNames struct {
	ServiceAccount     string
	ClusterRole        string
	ClusterRoleBinding string
	Role               string
	RoleBinding        string
}
