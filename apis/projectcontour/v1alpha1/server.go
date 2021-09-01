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

package v1alpha1

import (
	"fmt"
)

// XDSServerType is the type of xDS server implementation.
type XDSServerType string

const ContourServerType XDSServerType = "contour"
const EnvoyServerType XDSServerType = "envoy"

// Validate the xDS server type.
func (s XDSServerType) Validate() error {
	switch s {
	case ContourServerType, EnvoyServerType:
		return nil
	default:
		return fmt.Errorf("invalid xDS server type %q", s)
	}
}

// ServerParameters holds the config for the Contour xDS server.
type ServerParameters struct {
	// Defines the XDSServer to use for `contour serve`.
	// Defaults to "contour"
	// +kubebuilder:default=contour
	// +kubebuilder:validation:Enum=contour;envoy
	Type XDSServerType `json:"type,omitempty"`

	// Defines the xDS gRPC API address which Contour will serve.
	// Defaults to "0.0.0.0".
	//  +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="0.0.0.0"
	Address string `json:"address,omitempty"`

	// Defines the xDS gRPC API port which Contour will serve.
	// Defaults to 8001.
	//  +optional
	// +kubebuilder:default=8001
	Port int `json:"port,omitempty"`

	// Allow serving the xDS gRPC API without TLS.
	// Defaults to false.
	//  +optional
	// +kubebuilder:default=false
	Insecure bool `json:"insecure,omitempty"`

	// TLS holds TLS file config details.
	//  +optional
	TLS *TLS `json:"tls,omitempty"`

	// Health defines the endpoints Contour will serve to enable health checks.
	Health *Health `json:"health,omitempty"`

	// Metrics defines the endpoints Contour will serve to enable metrics.
	Metrics *Metrics `json:"metrics,omitempty"`
}
