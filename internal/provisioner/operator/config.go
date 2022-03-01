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

package operator

const (
	DefaultContourImage           = "ghcr.io/projectcontour/contour:main"
	DefaultEnvoyImage             = "docker.io/envoyproxy/envoy:v1.21.1"
	DefaultMetricsAddr            = ":8080"
	DefaultEnableLeaderElection   = false
	DefaultEnableLeaderElectionID = "0d879e31.projectcontour.io"
	DefaultGatewayControllerName  = "projectcontour.io/gateway-provisioner"
)

// Config is configuration of the operator.
type Config struct {
	// ContourImage is the container image for the Contour container(s) managed
	// by the operator.
	ContourImage string

	// EnvoyImage is the container image for the Envoy container(s) managed
	// by the operator.
	EnvoyImage string

	// MetricsBindAddress is the TCP address that the operator should bind to for
	// serving prometheus metrics. It can be set to "0" to disable the metrics serving.
	MetricsBindAddress string

	// LeaderElection determines whether or not to use leader election when starting
	// the operator.
	LeaderElection bool

	// LeaderElectionID determines the name of the configmap that leader election will
	// use for holding the leader lock.
	LeaderElectionID string

	// GatewayControllerName defines the controller string that this operator instance
	// will process GatewayClasses and Gateways for.
	GatewayControllerName string
}

// DefaultConfig returns an operator config using default values.
func DefaultConfig() *Config {
	return &Config{
		ContourImage:          DefaultContourImage,
		EnvoyImage:            DefaultEnvoyImage,
		MetricsBindAddress:    DefaultMetricsAddr,
		LeaderElection:        DefaultEnableLeaderElection,
		LeaderElectionID:      DefaultEnableLeaderElectionID,
		GatewayControllerName: DefaultGatewayControllerName,
	}
}
