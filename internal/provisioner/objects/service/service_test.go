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

package service

import (
	"fmt"
	"sort"
	"testing"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"
	objcontour "github.com/projectcontour/contour-operator/internal/objects/contour"
	objcfg "github.com/projectcontour/contour-operator/internal/objects/sharedconfig"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func checkServiceHasPort(t *testing.T, svc *corev1.Service, port int32) {
	t.Helper()

	for _, p := range svc.Spec.Ports {
		if p.Port == port {
			return
		}
	}
	t.Errorf("service is missing port %q", port)
}

func checkServiceHasNodeport(t *testing.T, svc *corev1.Service, port int32) {
	t.Helper()

	for _, p := range svc.Spec.Ports {
		if p.NodePort == port {
			return
		}
	}
	t.Errorf("service is missing nodeport %q", port)
}

func checkServiceHasTargetPort(t *testing.T, svc *corev1.Service, port int32) {
	t.Helper()

	intStrPort := intstr.IntOrString{IntVal: port}
	for _, p := range svc.Spec.Ports {
		if p.TargetPort == intStrPort {
			return
		}
	}
	t.Errorf("service is missing targetPort %d", port)
}

func checkServiceHasPortName(t *testing.T, svc *corev1.Service, name string) {
	t.Helper()

	for _, p := range svc.Spec.Ports {
		if p.Name == name {
			return
		}
	}
	t.Errorf("service is missing port name %q", name)
}

func checkServiceHasPortProtocol(t *testing.T, svc *corev1.Service, protocol corev1.Protocol) {
	t.Helper()

	for _, p := range svc.Spec.Ports {
		if p.Protocol == protocol {
			return
		}
	}
	t.Errorf("service is missing port protocol %q", protocol)
}

func checkServiceHasAnnotations(t *testing.T, svc *corev1.Service, expectedKeys ...string) {
	t.Helper()

	// get all of the actual annotation keys from the service
	var actualKeys []string
	for k := range svc.Annotations {
		actualKeys = append(actualKeys, k)
	}

	sort.Strings(actualKeys)
	sort.Strings(expectedKeys)

	// short-cut to an error if the slices are different lengths
	if len(expectedKeys) != len(actualKeys) {
		t.Errorf("expected service annotation keys %v, got %v", expectedKeys, actualKeys)
		return
	}

	// now that the slices are sorted & same length we can compare item by item
	for i, want := range expectedKeys {
		if actualKeys[i] != want {
			t.Errorf("expected service annotation keys %v, got %v", expectedKeys, actualKeys)
			return
		}
	}
}

func checkServiceHasType(t *testing.T, svc *corev1.Service, svcType corev1.ServiceType) {
	t.Helper()

	if svc.Spec.Type != svcType {
		t.Errorf("service is missing type %s", svcType)
	}
}

func checkServiceHasExternalTrafficPolicy(t *testing.T, svc *corev1.Service, policy corev1.ServiceExternalTrafficPolicyType) {
	t.Helper()

	if svc.Spec.ExternalTrafficPolicy != policy {
		t.Errorf("service is missing external traffic policy type %s", policy)
	}
}

func checkServiceHasLoadBalancerAddress(t *testing.T, svc *corev1.Service, address string) {
	t.Helper()

	if svc.Spec.LoadBalancerIP != address {
		t.Errorf("service is missing predefined load balancer ip %s", address)
	}
}

func TestDesiredContourService(t *testing.T) {
	name := "svc-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.LoadBalancerServicePublishingType,
	}
	cntr := objcontour.New(cfg)
	svc := DesiredContourService(cntr)
	xdsPort := objcfg.XDSPort
	checkServiceHasPort(t, svc, xdsPort)
	checkServiceHasTargetPort(t, svc, xdsPort)
	checkServiceHasPortName(t, svc, "xds")
	checkServiceHasPortProtocol(t, svc, corev1.ProtocolTCP)
}

func TestDesiredEnvoyService(t *testing.T) {
	name := "svc-test"
	loadBalancerAddress := "1.2.3.4"
	allocationIDs := []string{"eipalloc-0123456789", "eipalloc-1234567890"}
	resourceGroup := "contour-rg-test"
	subnet := "contour-subnet-test"
	cfg := objcontour.Config{
		Name:        name,
		Namespace:   fmt.Sprintf("%s-ns", name),
		SpecNs:      "projectcontour",
		RemoveNs:    false,
		NetworkType: operatorv1alpha1.NodePortServicePublishingType,
		NodePorts:   objcontour.MakeNodePorts(map[string]int{"http": 30081, "https": 30444}),
	}
	cntr := objcontour.New(cfg)
	svc := DesiredEnvoyService(cntr)
	checkServiceHasType(t, svc, corev1.ServiceTypeNodePort)
	checkServiceHasExternalTrafficPolicy(t, svc, corev1.ServiceExternalTrafficPolicyTypeLocal)
	checkServiceHasPort(t, svc, EnvoyServiceHTTPPort)
	checkServiceHasPort(t, svc, EnvoyServiceHTTPSPort)
	checkServiceHasNodeport(t, svc, 30081)
	checkServiceHasNodeport(t, svc, 30444)
	for _, port := range cntr.Spec.NetworkPublishing.Envoy.ContainerPorts {
		checkServiceHasTargetPort(t, svc, port.PortNumber)
	}
	checkServiceHasPortName(t, svc, "http")
	checkServiceHasPortName(t, svc, "https")
	checkServiceHasPortProtocol(t, svc, corev1.ProtocolTCP)

	// Check LB annotations for the different provider types, starting with AWS ELB (the default
	// if AWS provider params are not passed).
	cntr.Spec.NetworkPublishing.Envoy.Type = operatorv1alpha1.LoadBalancerServicePublishingType
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.Scope = operatorv1alpha1.ExternalLoadBalancer
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters.Type = operatorv1alpha1.AWSLoadBalancerProvider
	svc = DesiredEnvoyService(cntr)
	checkServiceHasType(t, svc, corev1.ServiceTypeLoadBalancer)
	checkServiceHasExternalTrafficPolicy(t, svc, corev1.ServiceExternalTrafficPolicyTypeLocal)
	checkServiceHasAnnotations(t, svc, awsLbBackendProtoAnnotation, awsLBProxyProtocolAnnotation)

	// Test proxy protocol for AWS Classic load balancer (when provider params are specified).
	elbParams := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type: operatorv1alpha1.AWSLoadBalancerProvider,
		AWS:  &operatorv1alpha1.AWSLoadBalancerParameters{Type: operatorv1alpha1.AWSClassicLoadBalancer},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = elbParams
	svc = DesiredEnvoyService(cntr)
	checkServiceHasAnnotations(t, svc, awsLbBackendProtoAnnotation, awsLBProxyProtocolAnnotation)

	// Check AWS NLB load balancer type.
	nlbParams := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type: operatorv1alpha1.AWSLoadBalancerProvider,
		AWS:  &operatorv1alpha1.AWSLoadBalancerParameters{Type: operatorv1alpha1.AWSNetworkLoadBalancer, AllocationIDs: allocationIDs},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = nlbParams
	svc = DesiredEnvoyService(cntr)
	// NLBs should not have PROXY protocol or backend protocol annotations.
	checkServiceHasAnnotations(t, svc, awsLBTypeAnnotation, awsLBAllocationIDsAnnotation)

	// Check Azure external load balancer type.
	azureParams := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type:  operatorv1alpha1.AzureLoadBalancerProvider,
		Azure: &operatorv1alpha1.AzureLoadBalancerParameters{Address: &loadBalancerAddress, ResourceGroup: &resourceGroup},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = azureParams
	svc = DesiredEnvoyService(cntr)
	checkServiceHasLoadBalancerAddress(t, svc, loadBalancerAddress)
	checkServiceHasAnnotations(t, svc, azureLBResourceGroupAnnotation)

	// Check GCP external load balancer type.
	gcpParams := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type: operatorv1alpha1.GCPLoadBalancerProvider,
		GCP:  &operatorv1alpha1.GCPLoadBalancerParameters{Address: &loadBalancerAddress},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = gcpParams
	svc = DesiredEnvoyService(cntr)
	checkServiceHasLoadBalancerAddress(t, svc, loadBalancerAddress)

	// Test an internal ELB
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.Scope = operatorv1alpha1.InternalLoadBalancer
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = elbParams
	svc = DesiredEnvoyService(cntr)
	checkServiceHasAnnotations(t, svc, awsInternalLBAnnotation, awsLbBackendProtoAnnotation, awsLBProxyProtocolAnnotation)

	// Test an internal Azure LB.
	azureParamsInternal := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type:  operatorv1alpha1.AzureLoadBalancerProvider,
		Azure: &operatorv1alpha1.AzureLoadBalancerParameters{Address: &loadBalancerAddress, Subnet: &subnet},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = azureParamsInternal
	svc = DesiredEnvoyService(cntr)
	checkServiceHasLoadBalancerAddress(t, svc, loadBalancerAddress)
	checkServiceHasAnnotations(t, svc, azureInternalLBAnnotation, azureLBSubnetAnnotation)

	// Test an internal GCP LB.
	gcpParamsInternal := operatorv1alpha1.ProviderLoadBalancerParameters{
		Type: operatorv1alpha1.GCPLoadBalancerProvider,
		GCP:  &operatorv1alpha1.GCPLoadBalancerParameters{Address: &loadBalancerAddress, Subnet: &subnet},
	}
	cntr.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters = gcpParamsInternal
	svc = DesiredEnvoyService(cntr)
	checkServiceHasLoadBalancerAddress(t, svc, loadBalancerAddress)
	checkServiceHasAnnotations(t, svc, gcpLBTypeAnnotation, gcpLBTypeAnnotationLegacy, gcpLBSubnetAnnotation)

	// Set network publishing type to ClusterIPService and verify the service type is as expected.
	cntr.Spec.NetworkPublishing.Envoy.Type = operatorv1alpha1.ClusterIPServicePublishingType
	svc = DesiredEnvoyService(cntr)
	checkServiceHasType(t, svc, corev1.ServiceTypeClusterIP)
	checkServiceHasAnnotations(t, svc) // passing no keys means we expect no annotations
}
