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

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

// GetConditionFor returns the a pointer to the condition for a given type,
// or nil if there are none currently present.
func (status *ExtensionServiceStatus) GetConditionFor(condType string) *contour_api_v1.DetailedCondition {
	for i, cond := range status.Conditions {
		if cond.Type == condType {
			return &status.Conditions[i]
		}
	}

	return nil
}

// Validate configuration that is not already covered by CRD validation.
func (c *ContourConfigurationSpec) Validate() error {
	if err := endpointsInConfict(c.Health, c.Metrics); err != nil {
		return fmt.Errorf("invalid contour configuration: %v", err)
	}
	return c.Envoy.Validate()
}

// Validate configuration that cannot be handled with CRD validation.
func (e *EnvoyConfig) Validate() error {
	if err := endpointsInConfict(e.Health, e.Metrics); err != nil {
		return fmt.Errorf("invalid envoy configuration: %v", err)
	}
	return nil
}

// endpointsInConfict returns error if different protocol are configured to use single port.
func endpointsInConfict(health *HealthConfig, metrics *MetricsConfig) error {
	if metrics.TLS != nil && health.Address == metrics.Address && health.Port == metrics.Port {
		return fmt.Errorf("cannot use single port for health over HTTP and metrics over HTTPS")
	}
	return nil
}
