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
	"strings"
)

// GatewayParameters holds the config for Gateway API controllers.
type GatewayParameters struct {
	// ControllerName is used to determine whether Contour should reconcile a
	// GatewayClass. The string takes the form of "projectcontour.io/<namespace>/contour".
	// If unset, the gatewayclass controller will not be started.
	ControllerName string `json:"controllerName,omitempty"`
}

func (g *GatewayParameters) Validate() error {

	var errorString string
	if g == nil {
		return nil
	}

	if len(g.ControllerName) == 0 {
		if len(errorString) > 0 {
			errorString += ","
		}
		errorString = strings.TrimSpace(fmt.Sprintf("%s controllerName required", errorString))
	}

	if len(errorString) > 0 {
		return fmt.Errorf("invalid Gateway parameters specified: %s", errorString)
	}
	return nil
}
