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

package validation

import (
	"context"
	"fmt"
	"net"

	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/slice"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Contour returns true if contour is valid.
func Contour(ctx context.Context, cli client.Client, contour *model.Contour) error {
	if err := ContainerPorts(contour); err != nil {
		return err
	}

	if contour.Spec.NetworkPublishing.Envoy.Type == model.NodePortServicePublishingType {
		if err := NodePorts(contour); err != nil {
			return err
		}
	}

	if contour.Spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType {
		if err := LoadBalancerAddress(contour); err != nil {
			return err
		}
		if err := LoadBalancerProvider(contour); err != nil {
			return err
		}
	}

	return nil
}

// ContainerPorts validates container ports of contour, returning an
// error if the container ports do not meet the API specification.
func ContainerPorts(contour *model.Contour) error {
	var numsFound []int32
	var namesFound []string
	httpFound := false
	httpsFound := false
	for _, port := range contour.Spec.NetworkPublishing.Envoy.ContainerPorts {
		if len(numsFound) > 0 && slice.ContainsInt32(numsFound, port.PortNumber) {
			return fmt.Errorf("duplicate container port number %d", port.PortNumber)
		}
		numsFound = append(numsFound, port.PortNumber)
		if len(namesFound) > 0 && slice.ContainsString(namesFound, port.Name) {
			return fmt.Errorf("duplicate container port name %q", port.Name)
		}
		namesFound = append(namesFound, port.Name)
		switch {
		case port.Name == "http":
			httpFound = true
		case port.Name == "https":
			httpsFound = true
		}
	}
	if httpFound && httpsFound {
		return nil
	}
	return fmt.Errorf("http and https container ports are unspecified")
}

// NodePorts validates nodeports of contour, returning an error if the nodeports
// do not meet the API specification.
func NodePorts(contour *model.Contour) error {
	ports := contour.Spec.NetworkPublishing.Envoy.NodePorts
	if ports == nil {
		// When unspecified, API server will auto-assign port numbers.
		return nil
	}
	for _, p := range ports {
		if p.Name != "http" && p.Name != "https" {
			return fmt.Errorf("invalid port name %q; only \"http\" and \"https\" are supported", p.Name)
		}
	}
	if ports[0].Name == ports[1].Name {
		return fmt.Errorf("duplicate nodeport names detected")
	}
	if ports[0].PortNumber != nil && ports[1].PortNumber != nil {
		if ports[0].PortNumber == ports[1].PortNumber {
			return fmt.Errorf("duplicate nodeport port numbers detected")
		}
	}

	return nil
}

// LoadBalancerAddress validates LoadBalancer "address" parameter of contour, returning an
// error if "address" does not meet the API specification.
func LoadBalancerAddress(contour *model.Contour) error {
	if contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type == model.AzureLoadBalancerProvider &&
		contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure != nil &&
		contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure.Address != nil {
		validationIP := net.ParseIP(*contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure.Address)
		if validationIP == nil {
			return fmt.Errorf("wrong LoadBalancer address format, should be string with IPv4 or IPv6 format")
		}
	} else if contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type == model.GCPLoadBalancerProvider &&
		contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP != nil &&
		contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Address != nil {
		validationIP := net.ParseIP(*contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP.Address)
		if validationIP == nil {
			return fmt.Errorf("wrong LoadBalancer address format, should be string with IPv4 or IPv6 format")
		}
	}

	return nil
}

// LoadBalancerProvider validates LoadBalancer provider parameters of contour, returning
// and error if parameters for different provider are specified the for the one specified
// with "type" parameter.
func LoadBalancerProvider(contour *model.Contour) error {
	switch contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type {
	case model.AWSLoadBalancerProvider:
		if contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure != nil ||
			contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP != nil {
			return fmt.Errorf("aws provider chosen, other providers parameters should not be specified")
		}
	case model.AzureLoadBalancerProvider:
		if contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.AWS != nil ||
			contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.GCP != nil {
			return fmt.Errorf("azure provider chosen, other providers parameters should not be specified")
		}
	case model.GCPLoadBalancerProvider:
		if contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.AWS != nil ||
			contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Azure != nil {
			return fmt.Errorf("gcp provider chosen, other providers parameters should not be specified")
		}
	}

	return nil
}
