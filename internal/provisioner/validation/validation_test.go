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

package validation_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/validation"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	envoyInsecureContainerPort = int32(8080)
	envoySecureContainerPort   = int32(8443)
)

func TestContainerPorts(t *testing.T) {
	testCases := []struct {
		description string
		ports       []model.ContainerPort
		expected    bool
	}{
		{
			description: "default http and https port",
			expected:    true,
		},
		{
			description: "non-default http and https ports",
			ports: []model.ContainerPort{
				{
					Name:       "http",
					PortNumber: int32(8081),
				},
				{
					Name:       "https",
					PortNumber: int32(8444),
				},
			},
			expected: true,
		},
		{
			description: "duplicate port names",
			ports: []model.ContainerPort{
				{
					Name:       "http",
					PortNumber: envoyInsecureContainerPort,
				},
				{
					Name:       "http",
					PortNumber: envoySecureContainerPort,
				},
			},
			expected: false,
		},
		{
			description: "duplicate port numbers",
			ports: []model.ContainerPort{
				{
					Name:       "http",
					PortNumber: envoyInsecureContainerPort,
				},
				{
					Name:       "https",
					PortNumber: envoyInsecureContainerPort,
				},
			},
			expected: false,
		},
		{
			description: "only http port specified",
			ports: []model.ContainerPort{
				{
					Name:       "http",
					PortNumber: envoyInsecureContainerPort,
				},
			},
			expected: false,
		},
		{
			description: "only https port specified",
			ports: []model.ContainerPort{
				{
					Name:       "https",
					PortNumber: envoySecureContainerPort,
				},
			},
			expected: false,
		},
		{
			description: "empty ports",
			ports:       []model.ContainerPort{},
			expected:    false,
		},
	}

	name := "test-validation"
	cntr := &model.Contour{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fmt.Sprintf("%s-ns", name),
		},
		Spec: model.ContourSpec{
			NetworkPublishing: model.NetworkPublishing{
				Envoy: model.EnvoyNetworkPublishing{
					Type: model.LoadBalancerServicePublishingType,
					ContainerPorts: []model.ContainerPort{
						{
							Name:       "http",
							PortNumber: int32(8080),
						},
						{
							Name:       "https",
							PortNumber: int32(8443),
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if tc.ports != nil {
			cntr.Spec.NetworkPublishing.Envoy.ContainerPorts = tc.ports
		}
		err := validation.ContainerPorts(cntr)
		if err != nil && tc.expected {
			t.Fatalf("%q: failed with error: %#v", tc.description, err)
		}
		if err == nil && !tc.expected {
			t.Fatalf("%q: expected to fail but received no error", tc.description)
		}
	}
}

func TestLoadBalancerIP(t *testing.T) {
	testCases := []struct {
		description string
		address     string
		expected    bool
	}{
		{
			description: "default load balancer type service without IP specified",
			expected:    true,
		},
		{
			description: "user-specified load balancer IPv4 address",
			address:     "1.2.3.4",
			expected:    true,
		},
		{
			description: "user-specified load balancer IPv6 address",
			address:     "2607:f0d0:1002:51::4",
			expected:    true,
		},
		{
			description: "invalid IPv4 address",
			address:     "1.2..4",
			expected:    false,
		},
		{
			description: "invalid IPv6 address",
			address:     "2607:f0d0:1002:51:::4",
			expected:    false,
		},
	}

	name := "test-validation"
	cntr := &model.Contour{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fmt.Sprintf("%s-ns", name),
		},
		Spec: model.ContourSpec{
			NetworkPublishing: model.NetworkPublishing{
				Envoy: model.EnvoyNetworkPublishing{
					Type: model.LoadBalancerServicePublishingType,
					LoadBalancer: model.LoadBalancerStrategy{
						Scope: "External",
						ProviderParameters: model.ProviderLoadBalancerParameters{
							Type: "GCP",
							GCP:  &model.GCPLoadBalancerParameters{},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if tc.address != "" {
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Address = &tc.address
		}

		err := validation.LoadBalancerAddress(cntr)
		if err != nil && tc.expected {
			t.Fatalf("%q: failed with error: %#v", tc.description, err)
		}
		if err == nil && !tc.expected {
			t.Fatalf("%q: expected to fail but received no error", tc.description)
		}
	}
}

func TestLoadBalancerProvider(t *testing.T) {
	testCases := []struct {
		description        string
		provider           model.LoadBalancerProviderType
		additionalProvider model.LoadBalancerProviderType
		expected           bool
	}{
		{
			description: "default load balancer parameters",
			expected:    true,
		},
		{
			description:        "aws provider with azure provider parameters specified",
			provider:           "AWS",
			additionalProvider: "Azure",
			expected:           false,
		},
		{
			description:        "aws provider with gcp provider parameters specified",
			provider:           "AWS",
			additionalProvider: "GCP",
			expected:           false,
		},
		{
			description:        "azure provider with aws provider parameters specified",
			provider:           "Azure",
			additionalProvider: "AWS",
			expected:           false,
		},
		{
			description:        "azure provider with gcp provider parameters specified",
			provider:           "Azure",
			additionalProvider: "GCP",
			expected:           false,
		},
		{
			description:        "gcp provider with aws provider parameters specified",
			provider:           "GCP",
			additionalProvider: "AWS",
			expected:           false,
		},
		{
			description:        "gcp provider with azure provider parameters specified",
			provider:           "GCP",
			additionalProvider: "Azure",
			expected:           false,
		},
	}

	name := "test-validation"
	testString := "projectcontour"
	cntr := &model.Contour{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fmt.Sprintf("%s-ns", name),
		},
		Spec: model.ContourSpec{
			NetworkPublishing: model.NetworkPublishing{
				Envoy: model.EnvoyNetworkPublishing{
					Type: model.LoadBalancerServicePublishingType,
					LoadBalancer: model.LoadBalancerStrategy{
						Scope: "External",
						ProviderParameters: model.ProviderLoadBalancerParameters{
							AWS:   &model.AWSLoadBalancerParameters{},
							Azure: &model.AzureLoadBalancerParameters{},
							GCP:   &model.GCPLoadBalancerParameters{},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		switch tc.provider {
		case "AWS":
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = model.AWSLoadBalancerProvider
			switch tc.additionalProvider {
			case "Azure":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure.Subnet = &testString
			case "GCP":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Subnet = &testString
			}
		case "Azure":
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = model.AzureLoadBalancerProvider
			switch tc.additionalProvider {
			case "AWS":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.AWS.AllocationIDs = strings.Split(testString, "")
			case "GCP":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Subnet = &testString
			}
		case "GCP":
			cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = model.GCPLoadBalancerProvider
			switch tc.additionalProvider {
			case "AWS":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.AWS.AllocationIDs = strings.Split(testString, "")
			case "Azure":
				cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure.Subnet = &testString
			}
		}
		err := validation.LoadBalancerProvider(cntr)
		if err != nil && tc.expected {
			t.Fatalf("%q: failed with error: %#v", tc.description, err)
		}
		if err == nil && !tc.expected {
			t.Fatalf("%q: expected to fail but received no error", tc.description)
		}
	}
}

func TestNodePorts(t *testing.T) {
	httpPort := int32(30080)
	httpsPort := int32(30443)

	testCases := []struct {
		description string
		ports       []model.NodePort
		expected    bool
	}{
		{
			description: "default http and https nodeports",
			expected:    true,
		},
		{
			description: "user-specified http and https nodeports",
			ports: []model.NodePort{
				{
					Name:       "http",
					PortNumber: &httpPort,
				},
				{
					Name:       "https",
					PortNumber: &httpsPort,
				},
			},
			expected: true,
		},
		{
			description: "invalid port name",
			ports: []model.NodePort{
				{
					Name:       "http",
					PortNumber: &httpPort,
				},
				{
					Name:       "foo",
					PortNumber: &httpsPort,
				},
			},
			expected: false,
		},
		{
			description: "auto-assigned https port number",
			ports: []model.NodePort{
				{
					Name:       "http",
					PortNumber: &httpPort,
				},
				{
					Name: "https",
				},
			},
			expected: true,
		},
		{
			description: "auto-assigned http and https port numbers",
			ports: []model.NodePort{
				{
					Name: "http",
				},
				{
					Name: "https",
				},
			},
			expected: true,
		},
		{
			description: "duplicate nodeport names",
			ports: []model.NodePort{
				{
					Name:       "http",
					PortNumber: &httpPort,
				},
				{
					Name:       "http",
					PortNumber: &httpsPort,
				},
			},
			expected: false,
		},
		{
			description: "duplicate nodeport numbers",
			ports: []model.NodePort{
				{
					Name:       "http",
					PortNumber: &httpPort,
				},
				{
					Name:       "https",
					PortNumber: &httpPort,
				},
			},
			expected: false,
		},
	}

	name := "test-validation"
	cntr := &model.Contour{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fmt.Sprintf("%s-ns", name),
		},
		Spec: model.ContourSpec{
			NetworkPublishing: model.NetworkPublishing{
				Envoy: model.EnvoyNetworkPublishing{
					Type: model.NodePortServicePublishingType,
					NodePorts: []model.NodePort{
						{
							Name:       "http",
							PortNumber: &httpPort,
						},
						{
							Name:       "https",
							PortNumber: &httpsPort,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if tc.ports != nil {
			cntr.Spec.NetworkPublishing.Envoy.NodePorts = tc.ports
		}
		err := validation.NodePorts(cntr)
		if err != nil && tc.expected {
			t.Fatalf("%q: failed with error: %#v", tc.description, err)
		}
		if err == nil && !tc.expected {
			t.Fatalf("%q: expected to fail but received no error", tc.description)
		}
	}
}
