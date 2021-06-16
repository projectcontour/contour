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

package gateway

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// OthersExistInNs lists Gateway objects in the same namespace as gw,
// returning an error if any exist.
func OthersExistInNs(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway) error {
	gwList, err := OthersExist(ctx, cli, gw)
	if err != nil {
		return err
	}
	if gwList != nil {
		for _, item := range gwList.Items {
			if item.Namespace == gw.Namespace && item.Name != gw.Name {
				return fmt.Errorf("gateway %s found in namespace %s", item.Name, gw.Namespace)
			}
		}
	}
	return nil
}

// OthersRefClass returns true if other gateways have the same gatewayClassName as gw.
func OthersRefClass(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway) (bool, error) {
	gwList, err := OthersExist(ctx, cli, gw)
	if err != nil {
		return false, err
	}
	if gwList != nil {
		for _, item := range gwList.Items {
			if item.Namespace != gw.Namespace &&
				item.Name != gw.Name &&
				item.Spec.GatewayClassName == gw.Spec.GatewayClassName {
				return true, nil
			}
		}
	}
	return false, nil
}

// OthersExist lists Gateway objects in all namespaces, returning the list
// if any exist other than gw.
func OthersExist(ctx context.Context, cli client.Client, gw *gatewayapi_v1alpha1.Gateway) (*gatewayapi_v1alpha1.GatewayList, error) {
	gwList := &gatewayapi_v1alpha1.GatewayList{}
	if err := cli.List(ctx, gwList); err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}
	if len(gwList.Items) == 0 || len(gwList.Items) == 1 && gwList.Items[0].Name == gw.Name {
		return nil, nil
	}
	return gwList, nil
}
