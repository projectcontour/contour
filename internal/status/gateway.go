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

package status

import (
	"context"
	"fmt"

	api_equality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// SyncGateway computes the current status of gw and updates status based on
// any changes since last sync.
func SyncGateway(ctx context.Context, cli client.Client, gw *gateway_v1alpha1.Gateway, errs field.ErrorList) error {
	updated := gw.DeepCopy()

	updated.Status.Conditions = mergeConditions(updated.Status.Conditions, computeGatewayScheduledCondition(errs))

	// Update status if current does not match desired.
	if !api_equality.Semantic.DeepEqual(gw.Status, updated.Status) {
		if err := cli.Status().Update(ctx, updated); err != nil {
			return fmt.Errorf("failed to update gateway %s/%s status: %w", updated.Namespace, updated.Name, err)
		}
	}

	return nil
}
