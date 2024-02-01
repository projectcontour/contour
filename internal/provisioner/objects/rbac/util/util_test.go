package util

import (
	"reflect"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

func TestFilterResources(t *testing.T) {
	testCases := []struct {
		description      string
		disabledFeatures []contourv1.Feature
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
			disabledFeatures: []contourv1.Feature{"extensionservices"},
			expectedList:     []string{"httpproxies", "tlscertificatedelegations", "contourconfigurations"},
		},
		{
			description:      "disable extensionservices, filter status",
			resourceList:     []string{"httpproxies/status", "extensionservices/status", "contourconfigurations/status"},
			disabledFeatures: []contourv1.Feature{"extensionservices"},
			expectedList:     []string{"httpproxies/status", "contourconfigurations/status"},
		},
		{
			description:      "disable tlsroutes",
			resourceList:     []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"},
			disabledFeatures: []contourv1.Feature{"tlsroutes"},
			expectedList:     []string{"gateways", "httproutes", "grpcroutes", "tcproutes", "referencegrants"},
		},
		{
			description:      "disable non existance abc",
			resourceList:     []string{"gateways", "httproutes", "tlsroutes", "grpcroutes", "tcproutes", "referencegrants"},
			disabledFeatures: []contourv1.Feature{"abc"},
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
