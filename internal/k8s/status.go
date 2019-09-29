// Copyright © 2019 VMware
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

// Package k8s contains helpers for setting the IngressRoute status
package k8s

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	clientset "github.com/projectcontour/contour/apis/generated/clientset/versioned"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CRDStatus allows for updating the object's Status field
type CRDStatus struct {
	Client clientset.Interface
}

// SetStatus sets the IngressRoute status field to an Valid or Invalid status
func (irs *CRDStatus) SetStatus(status, desc string, existing interface{}) error {
	switch exist := existing.(type) {
	case *ingressroutev1.IngressRoute:
		// Check if update needed by comparing status & desc
		if irs.updateNeeded(status, desc, exist.Status) {
			updated := exist.DeepCopy()
			updated.Status = projcontour.Status{
				CurrentStatus: status,
				Description:   desc,
			}
			return irs.setIngressRouteStatus(exist, updated)
		}
	case *projcontour.HTTPProxy:
		// Check if update needed by comparing status & desc
		if irs.updateNeeded(status, desc, exist.Status) {
			updated := exist.DeepCopy()
			updated.Status = projcontour.Status{
				CurrentStatus: status,
				Description:   desc,
			}
			return irs.setHTTPProxyStatus(exist, updated)
		}
	}
	return nil
}

func (irs *CRDStatus) updateNeeded(status, desc string, existing projcontour.Status) bool {
	if existing.CurrentStatus != status || existing.Description != desc {
		return true
	}
	return false
}

func (irs *CRDStatus) setIngressRouteStatus(existing, updated *ingressroutev1.IngressRoute) error {
	existingBytes, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	// Need to set the resource version of the updated endpoints to the resource
	// version of the current service. Otherwise, the resulting patch does not
	// have a resource version, and the server complains.
	updated.ResourceVersion = existing.ResourceVersion
	updatedBytes, err := json.Marshal(updated)
	if err != nil {
		return err
	}
	patchBytes, err := jsonpatch.CreateMergePatch(existingBytes, updatedBytes)
	if err != nil {
		return err
	}

	_, err = irs.Client.ContourV1beta1().IngressRoutes(existing.GetNamespace()).Patch(existing.GetName(), types.MergePatchType, patchBytes)
	return err
}

func (irs *CRDStatus) setHTTPProxyStatus(existing, updated *projcontour.HTTPProxy) error {
	existingBytes, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	// Need to set the resource version of the updated endpoints to the resource
	// version of the current service. Otherwise, the resulting patch does not
	// have a resource version, and the server complains.
	updated.ResourceVersion = existing.ResourceVersion
	updatedBytes, err := json.Marshal(updated)
	if err != nil {
		return err
	}
	patchBytes, err := jsonpatch.CreateMergePatch(existingBytes, updatedBytes)
	if err != nil {
		return err
	}

	_, err = irs.Client.ProjectcontourV1().HTTPProxies(existing.GetNamespace()).Patch(existing.GetName(), types.MergePatchType, patchBytes)
	return err
}
