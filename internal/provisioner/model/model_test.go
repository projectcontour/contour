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

package model

import (
	"reflect"
	"testing"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func TestNamespacesToStrings(t *testing.T) {
	testCases := []struct {
		description   string
		namespaces    []contour_v1.Namespace
		expectStrings []string
	}{
		{
			description:   "no namespaces",
			namespaces:    []contour_v1.Namespace{},
			expectStrings: []string{},
		},
		{
			description:   "2 namespaces",
			namespaces:    []contour_v1.Namespace{"ns1", "ns2"},
			expectStrings: []string{"ns1", "ns2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if !reflect.DeepEqual(NamespacesToStrings(tc.namespaces), tc.expectStrings) {
				t.Errorf("expect converted strings %v is the same as %v", NamespacesToStrings(tc.namespaces), tc.expectStrings)
			}
		})
	}
}

func TestFeaturesToStrings(t *testing.T) {
	testCases := []struct {
		description   string
		features      []contour_v1.Feature
		expectStrings []string
	}{
		{
			description:   "no features",
			features:      []contour_v1.Feature{},
			expectStrings: []string{},
		},
		{
			description:   "2 features",
			features:      []contour_v1.Feature{"tlsroutes", "grpcroutes"},
			expectStrings: []string{"tlsroutes", "grpcroutes"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			if !reflect.DeepEqual(FeaturesToStrings(tc.features), tc.expectStrings) {
				t.Errorf("expect converted strings %v is the same as %v", FeaturesToStrings(tc.features), tc.expectStrings)
			}
		})
	}
}
