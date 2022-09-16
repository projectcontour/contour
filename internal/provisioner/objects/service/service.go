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
	"context"
	"fmt"
	"strings"

	"github.com/projectcontour/contour/internal/provisioner/equality"
	"github.com/projectcontour/contour/internal/provisioner/labels"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	"github.com/projectcontour/contour/internal/provisioner/objects/dataplane"
	"github.com/projectcontour/contour/internal/provisioner/objects/deployment"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// awsLbBackendProtoAnnotation is a Service annotation that places the AWS ELB into
	// "TCP" mode so that it does not do HTTP negotiation for HTTPS connections at the
	// ELB edge. The downside of this is the remote IP address of all connections will
	// appear to be the internal address of the ELB.
	// TODO [danehans]: Make proxy protocol configurable or automatically enabled. See
	// https://github.com/projectcontour/contour-operator/issues/49 for details.
	awsLbBackendProtoAnnotation = "service.beta.kubernetes.io/aws-load-balancer-backend-protocol"
	// awsLBTypeAnnotation is a Service annotation used to specify an AWS load
	// balancer type. See the following for additional details:
	// https://kubernetes.io/docs/concepts/services-networking/service/#aws-nlb-support
	awsLBTypeAnnotation = "service.beta.kubernetes.io/aws-load-balancer-type"
	// awsLBProxyProtocolAnnotation is used to enable the PROXY protocol for an AWS Classic
	// load balancer. For additional details, see:
	// https://kubernetes.io/docs/concepts/services-networking/service/#proxy-protocol-support-on-aws
	awsLBProxyProtocolAnnotation = "service.beta.kubernetes.io/aws-load-balancer-proxy-protocol"
	// awsLBAllocationIDsAnnotation is a Service annotation that provides capability to
	// assign Load Balancer IP based on Allocation IDs of AWS Elastic IP resources when
	// load balancer scope is set to "External"
	awsLBAllocationIDsAnnotation = "service.beta.kubernetes.io/aws-load-balancer-eip-allocations"
	// awsInternalLBAnnotation is the annotation used on a service to specify an AWS
	// load balancer as being internal.
	awsInternalLBAnnotation = "service.beta.kubernetes.io/aws-load-balancer-internal"
	// azureLBResourceGroupAnnotation is a Service annotation that provides capability
	// to assign Load Balancer IP based on Public IP Azure resource that resides in
	// different resource group as AKS cluster when load balancer scope is set to "External".
	azureLBResourceGroupAnnotation = "service.beta.kubernetes.io/azure-load-balancer-resource-group"
	// azureLBSubnetAnnotation is a Service annotation that provides capability to assign
	// Load Balancer IP based on desired subnet when load balancer scope is set to "Internal".
	azureLBSubnetAnnotation = "service.beta.kubernetes.io/azure-load-balancer-internal-subnet"
	// azureInternalLBAnnotation is the annotation used on a service to specify an Azure
	// load balancer as being internal.
	azureInternalLBAnnotation = "service.beta.kubernetes.io/azure-load-balancer-internal"
	// gcpLBSubnetAnnotation is a Service annotation that provides capability to assign
	// Load Balancer IP to specified subnet when load balancer scope is set to "Internal".
	gcpLBSubnetAnnotation = "networking.gke.io/internal-load-balancer-subnet"
	// gcpLBTypeAnnotationLegacy is the annotation used on a service to specify a GCP load balancer
	// type for GKE version earlier then 1.17.
	gcpLBTypeAnnotationLegacy = "cloud.google.com/load-balancer-type"
	// gcpLBTypeAnnotation is the annotation used on a service to specify a GCP load balancer
	// type for GKE version 1.17 and later.
	gcpLBTypeAnnotation = "networking.gke.io/load-balancer-type"
	// EnvoyServiceHTTPPort is the HTTP port number of the Envoy service.
	EnvoyServiceHTTPPort = int32(80)
	// EnvoyServiceHTTPSPort is the HTTPS port number of the Envoy service.
	EnvoyServiceHTTPSPort = int32(443)
	// EnvoyNodePortHTTPPort is the NodePort port number for Envoy's HTTP service. For NodePort
	// details see: https://kubernetes.io/docs/concepts/services-networking/service/#nodeport
	EnvoyNodePortHTTPPort = int32(30080)
	// EnvoyNodePortHTTPSPort is the NodePort port number for Envoy's HTTPS service. For NodePort
	// details see: https://kubernetes.io/docs/concepts/services-networking/service/#nodeport
	EnvoyNodePortHTTPSPort = int32(30443)
)

var (
	// InternalLBAnnotations maps cloud providers to the provider's annotation
	// key/value pair used for managing an internal load balancer. For additional
	// details see:
	//  https://kubernetes.io/docs/concepts/services-networking/service/#internal-load-balancer
	//
	InternalLBAnnotations = map[model.LoadBalancerProviderType]map[string]string{
		model.AWSLoadBalancerProvider: {
			awsInternalLBAnnotation: "true",
		},
		model.AzureLoadBalancerProvider: {
			// Azure load balancers are not customizable and are set to (2 fail @ 5s interval, 2 healthy)
			azureInternalLBAnnotation: "true",
		},
		model.GCPLoadBalancerProvider: {
			gcpLBTypeAnnotation:       "Internal",
			gcpLBTypeAnnotationLegacy: "Internal",
		},
	}
)

// EnsureContourService ensures that a Contour Service exists for the given contour.
func EnsureContourService(ctx context.Context, cli client.Client, contour *model.Contour) error {
	desired := DesiredContourService(contour)
	current, err := currentService(ctx, cli, contour.Namespace, contour.ContourServiceName())
	if err != nil {
		if errors.IsNotFound(err) {
			return createService(ctx, cli, desired)
		}
		return fmt.Errorf("failed to get service %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if err := updateContourServiceIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to update service %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

// EnsureEnvoyService ensures that an Envoy Service exists for the given contour.
func EnsureEnvoyService(ctx context.Context, cli client.Client, contour *model.Contour) error {
	desired := DesiredEnvoyService(contour)
	current, err := currentService(ctx, cli, contour.Namespace, contour.EnvoyServiceName())
	if err != nil {
		if errors.IsNotFound(err) {
			return createService(ctx, cli, desired)
		}
		return fmt.Errorf("failed to get service %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if err := updateEnvoyServiceIfNeeded(ctx, cli, contour, current, desired); err != nil {
		return fmt.Errorf("failed to update service %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func ensureServiceDeleted(ctx context.Context, cli client.Client, contour *model.Contour, name string) error {
	getter := func(ctx context.Context, cli client.Client, namespace, name2 string) (client.Object, error) {
		return currentService(ctx, cli, namespace, name2)
	}
	return objects.EnsureObjectDeleted(ctx, cli, contour, name, getter)
}

// EnsureContourServiceDeleted ensures that a Contour Service for the
// provided contour is deleted if Contour owner labels exist.
func EnsureContourServiceDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	return ensureServiceDeleted(ctx, cli, contour, contour.ContourServiceName())
}

// EnsureEnvoyServiceDeleted ensures that an Envoy Service for the
// provided contour is deleted.
func EnsureEnvoyServiceDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	return ensureServiceDeleted(ctx, cli, contour, contour.EnvoyServiceName())
}

// DesiredContourService generates the desired Contour Service for the given contour.
func DesiredContourService(contour *model.Contour) *corev1.Service {
	xdsPort := objects.XDSPort
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: contour.Namespace,
			Name:      contour.ContourServiceName(),
			Labels:    model.CommonLabels(contour),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "xds",
					Port:       xdsPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{IntVal: xdsPort},
				},
			},
			Selector:        deployment.ContourDeploymentPodSelector(contour).MatchLabels,
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	return svc
}

// DesiredEnvoyService generates the desired Envoy Service for the given contour.
func DesiredEnvoyService(contour *model.Contour) *corev1.Service {
	var ports []corev1.ServicePort

	// Match service ports up to container ports based on name (the names
	// are statically set by provisioner code to "http" and "https" so for
	// now we're logically guaranteed to find matches).
	for _, servicePort := range contour.Spec.NetworkPublishing.Envoy.ServicePorts {
		for _, containerPort := range contour.Spec.NetworkPublishing.Envoy.ContainerPorts {
			if servicePort.Name == containerPort.Name {
				ports = append(ports, corev1.ServicePort{
					Name:       servicePort.Name,
					Protocol:   corev1.ProtocolTCP,
					Port:       servicePort.PortNumber,
					TargetPort: intstr.IntOrString{IntVal: containerPort.PortNumber},
				})
				break
			}
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   contour.Namespace,
			Name:        contour.EnvoyServiceName(),
			Annotations: map[string]string{},
			Labels:      model.CommonLabels(contour),
		},
		Spec: corev1.ServiceSpec{
			Ports:           ports,
			Selector:        dataplane.EnvoyPodSelector(contour).MatchLabels,
			SessionAffinity: corev1.ServiceAffinityNone,
			LoadBalancerIP:  contour.Spec.NetworkPublishing.Envoy.LoadBalancer.LoadBalancerIP,
		},
	}

	providerParams := &contour.Spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters

	// Add AWS LB annotations based on the network publishing strategy and provider type.
	if contour.Spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType &&
		providerParams.Type == model.AWSLoadBalancerProvider {

		// Add the TCP backend protocol annotation for AWS classic load balancers.
		if isELB(providerParams) {
			svc.Annotations[awsLbBackendProtoAnnotation] = "tcp"
			svc.Annotations[awsLBProxyProtocolAnnotation] = "*"
		} else {
			// Annotate the service for an NLB.
			svc.Annotations[awsLBTypeAnnotation] = "nlb"
		}
	}

	// Add the AllocationIDs annotation if specified by AWS provider parameters.
	if allocationIDsNeeded(&contour.Spec) {
		svc.Annotations[awsLBAllocationIDsAnnotation] = strings.Join(providerParams.AWS.AllocationIDs, ",")
	}

	// Add the ResourceGroup annotation if specified by Azure provider parameters.
	if resourceGroupNeeded(&contour.Spec) {
		svc.Annotations[azureLBResourceGroupAnnotation] = *providerParams.Azure.ResourceGroup
	}

	// Add the Subnet annotation if specified by provider parameters.
	if subnetNeeded(&contour.Spec) {
		if providerParams.Type == model.AzureLoadBalancerProvider {
			svc.Annotations[azureLBSubnetAnnotation] = *providerParams.Azure.Subnet
		} else if providerParams.Type == model.GCPLoadBalancerProvider {
			svc.Annotations[gcpLBSubnetAnnotation] = *providerParams.GCP.Subnet
		}
	}

	// Add LoadBalancerIP parameter if specified by provider parameters.
	if loadBalancerAddressNeeded(&contour.Spec) {
		if providerParams.Type == model.AzureLoadBalancerProvider {
			svc.Spec.LoadBalancerIP = *providerParams.Azure.Address
		} else if providerParams.Type == model.GCPLoadBalancerProvider {
			svc.Spec.LoadBalancerIP = *providerParams.GCP.Address
		}
	}

	epType := contour.Spec.NetworkPublishing.Envoy.Type
	if epType == model.LoadBalancerServicePublishingType ||
		epType == model.NodePortServicePublishingType {
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
	}
	switch epType {
	case model.LoadBalancerServicePublishingType:
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		isInternal := contour.Spec.NetworkPublishing.Envoy.LoadBalancer.Scope == model.InternalLoadBalancer
		if isInternal {
			provider := providerParams.Type
			internalAnnotations := InternalLBAnnotations[provider]
			for name, value := range internalAnnotations {
				svc.Annotations[name] = value
			}
		}
	case model.NodePortServicePublishingType:
		svc.Spec.Type = corev1.ServiceTypeNodePort

		for _, p := range contour.Spec.NetworkPublishing.Envoy.NodePorts {
			if p.PortNumber == nil {
				continue
			}
			for i, q := range svc.Spec.Ports {
				if q.Name == p.Name {
					svc.Spec.Ports[i].NodePort = *p.PortNumber
				}
			}
		}

	case model.ClusterIPServicePublishingType:
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}

	if len(contour.Spec.NetworkPublishing.Envoy.ServiceAnnotations) > 0 {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}

		for k, v := range contour.Spec.NetworkPublishing.Envoy.ServiceAnnotations {
			svc.Annotations[k] = v
		}
	}

	return svc
}

// currentContourService returns the current Contour/Envoy Service for the provided contour.
func currentService(ctx context.Context, cli client.Client, namespace, name string) (*corev1.Service, error) {
	current := &corev1.Service{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	err := cli.Get(ctx, key, current)
	if err != nil {
		return nil, err
	}
	return current, nil
}

// createService creates a Service resource for the provided svc.
func createService(ctx context.Context, cli client.Client, svc *corev1.Service) error {
	if err := cli.Create(ctx, svc); err != nil {
		return fmt.Errorf("failed to create service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	return nil
}

// updateContourServiceIfNeeded updates a Contour Service if current does not match desired.
func updateContourServiceIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *corev1.Service) error {
	if !labels.Exist(current, model.OwnerLabels(contour)) {
		return nil
	}
	_, updated := equality.ClusterIPServiceChanged(current, desired)
	if !updated {
		return nil
	}
	if err := cli.Update(ctx, desired); err != nil {
		return fmt.Errorf("failed to update service %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	return nil

}

// updateEnvoyServiceIfNeeded updates an Envoy Service if current does not match desired,
// using contour to verify the existence of owner labels.
func updateEnvoyServiceIfNeeded(ctx context.Context, cli client.Client, contour *model.Contour, current, desired *corev1.Service) error {
	if !labels.Exist(current, model.OwnerLabels(contour)) {
		return nil
	}

	// Using the Service returned by the equality pkg instead of the desired
	// parameter since clusterIP is immutable.
	var updated *corev1.Service
	needed := false
	switch contour.Spec.NetworkPublishing.Envoy.Type {
	case model.NodePortServicePublishingType:
		updated, needed = equality.NodePortServiceChanged(current, desired)

	case model.ClusterIPServicePublishingType:
		updated, needed = equality.ClusterIPServiceChanged(current, desired)

	// Add additional network publishing types as they are introduced.
	default:
		// LoadBalancerService is the default network publishing type.
		updated, needed = equality.LoadBalancerServiceChanged(current, desired)
	}
	if needed {
		if err := cli.Update(ctx, updated); err != nil {
			return fmt.Errorf("failed to update service %s/%s: %w", desired.Namespace, desired.Name, err)
		}
	}
	return nil
}

// isELB returns true if params is an AWS Classic ELB.
func isELB(params *model.ProviderLoadBalancerParameters) bool {
	return params.Type == model.AWSLoadBalancerProvider &&
		(params.AWS == nil || params.AWS.Type == model.AWSClassicLoadBalancer)
}

// allocationIDsNeeded returns true if "service.beta.kubernetes.io/aws-load-balancer-eip-allocations"
// annotation is needed based on the provided spec.
func allocationIDsNeeded(spec *model.ContourSpec) bool {
	providerParams := &spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters

	return spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType &&
		spec.NetworkPublishing.Envoy.LoadBalancer.Scope == "External" &&
		providerParams.Type == model.AWSLoadBalancerProvider &&
		providerParams.AWS != nil &&
		providerParams.AWS.Type == model.AWSNetworkLoadBalancer &&
		providerParams.AWS.AllocationIDs != nil
}

// resourceGroupNeeded returns true if "service.beta.kubernetes.io/azure-load-balancer-resource-group"
// annotation is needed based on the provided spec.
func resourceGroupNeeded(spec *model.ContourSpec) bool {
	providerParams := &spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters

	return spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType &&
		providerParams.Type == model.AzureLoadBalancerProvider &&
		providerParams.Azure != nil &&
		spec.NetworkPublishing.Envoy.LoadBalancer.Scope == "External" &&
		providerParams.Azure.ResourceGroup != nil
}

// subnetNeeded returns true if "service.beta.kubernetes.io/azure-load-balancer-internal-subnet" or
// "networking.gke.io/internal-load-balancer-subnet" annotation is needed based
// on the provided spec.
func subnetNeeded(spec *model.ContourSpec) bool {
	providerParams := &spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters

	return spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType &&
		spec.NetworkPublishing.Envoy.LoadBalancer.Scope == "Internal" &&
		((providerParams.Type == model.AzureLoadBalancerProvider &&
			providerParams.Azure != nil &&
			providerParams.Azure.Subnet != nil) ||
			(providerParams.Type == model.GCPLoadBalancerProvider &&
				providerParams.GCP != nil &&
				providerParams.GCP.Subnet != nil))
}

// loadBalancerAddressNeeded returns true if LoadBalancerIP parameter of service
// is needed based on provided spec.
func loadBalancerAddressNeeded(spec *model.ContourSpec) bool {
	providerParams := &spec.NetworkPublishing.Envoy.LoadBalancer.ProviderParameters

	return spec.NetworkPublishing.Envoy.Type == model.LoadBalancerServicePublishingType &&
		((providerParams.Type == model.AzureLoadBalancerProvider &&
			providerParams.Azure != nil &&
			providerParams.Azure.Address != nil) ||
			(providerParams.Type == model.GCPLoadBalancerProvider &&
				providerParams.GCP != nil &&
				providerParams.GCP.Address != nil))
}
