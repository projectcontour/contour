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

package util

import (
	"reflect"
	"testing"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func TestFilterResources(t *testing.T) {
	testCases := []struct {
		description      string
		disabledFeatures []contour_v1.Feature
		resourceList     []string
		expectedList     []string
	}{
		{
			description:      "empty disabled features",
			resourceList:     []string{"httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"},
			disabledFeatures: nil,
			expectedList:     []string{"httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"},
		},
		{
			description:      "disable extensionservices",
			resourceList:     []string{"httpproxies", "tlscertificatedelegations", "extensionservices", "contourconfigurations"},
			disabledFeatures: []contour_v1.Feature{"extensionservices"},
			expectedList:     []string{"httpproxies", "tlscertificatedelegations", "contourconfigurations"},
		},
		{
			description:      "disable extensionservices, filter status",
			resourceList:     []string{"httpproxies/status", "extensionservices/status", "contourconfigurations/status"},
			disabledFeatures: []contour_v1.Feature{"extensionservices"},
			expectedList:     []string{"httpproxies/status", "contourconfigurations/status"},
		},
		{
			description:      "disable tlsroutes",
			resourceList:     []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"},
			disabledFeatures: []contour_v1.Feature{"tlsroutes"},
			expectedList:     []string{"gateways", "httproutes", "grpcroutes", "tcproutes", "referencegrants"},
		},
		{
			description:      "disable non-existence abc",
			resourceList:     []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"},
			disabledFeatures: []contour_v1.Feature{"abc"},
			expectedList:     []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			f := filterResources(tc.disabledFeatures, tc.resourceList...)
			if !reflect.DeepEqual(tc.expectedList, f) {
				t.Errorf("expect filtered list to be %v, but is %v",
					tc.expectedList, f)
			}
		})
	}
}
