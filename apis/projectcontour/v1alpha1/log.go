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

// LogLevel is the logging levels available.
type LogLevel string

const InfoLog LogLevel = "info"
const DebugLog LogLevel = "debug"

// Debug contains Contour specific troubleshooting options.
type Debug struct {
	// Defines the Contour debug address interface.
	//  +optional
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address,omitempty"`

	// Defines the xDS gRPC API port which Contour will serve.
	// Defaults to 8001.
	//  +optional
	// +kubebuilder:default=8001
	Port int `json:"port,omitempty"`

	// DebugLogLevel defines the log level which Contour will
	// use when outputting log information.
	// +optional
	// +kubebuilder:default=info
	// +kubebuilder:validation:Enum=info;debug
	DebugLogLevel LogLevel `json:"logLevel,omitempty"`

	// KubernetesDebugLogLevel defines the log level which Contour will
	// use when outputting Kubernetes specific log information.
	//
	// Details: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	KubernetesDebugLogLevel int `json:"kubernetesLogLevel,omitempty"`
}
