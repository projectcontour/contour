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

//go:build e2e
// +build e2e

package e2e

import contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"

// HTTPProxyValid returns true if the proxy has a .status.currentStatus
// of "valid".
func HTTPProxyValid(proxy *contourv1.HTTPProxy) bool {

	if proxy == nil {
		return false
	}

	if len(proxy.Status.Conditions) == 0 {
		return false
	}

	cond := proxy.Status.GetConditionFor("Valid")
	return cond.Status == "True"

}
