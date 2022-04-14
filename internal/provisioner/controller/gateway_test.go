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

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	contourv1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/gatewayapi"
	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestGatewayReconcile(t *testing.T) {
	const controller = "projectcontour.io/gateway-controller"

	reconcilableGatewayClass := func(name, controller string) *gatewayv1alpha2.GatewayClass {
		return &gatewayv1alpha2.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: gatewayv1alpha2.GatewayClassSpec{
				ControllerName: gatewayv1alpha2.GatewayController(controller),
			},
			// the fake client lets us create resources with a status set
			Status: gatewayv1alpha2.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1alpha2.GatewayClassReasonAccepted),
					},
				},
			},
		}
	}

	reconcilableGatewayClassWithParams := func(name, controller string) *gatewayv1alpha2.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayv1alpha2.ParametersReference{
			Group:     gatewayv1alpha2.Group(contourv1alpha1.GroupVersion.Group),
			Kind:      "ContourDeployment",
			Namespace: gatewayapi.NamespacePtr("projectcontour"),
			Name:      name + "-params",
		}
		return gc
	}

	reconcilableGatewayClassWithInvalidParams := func(name, controller string) *gatewayv1alpha2.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayv1alpha2.ParametersReference{
			Group:     gatewayv1alpha2.Group(contourv1alpha1.GroupVersion.Group),
			Kind:      "InvalidKind",
			Namespace: gatewayapi.NamespacePtr("projectcontour"),
			Name:      name + "-params",
		}
		return gc
	}

	tests := map[string]struct {
		gatewayClass       *gatewayv1alpha2.GatewayClass
		gatewayClassParams *contourv1alpha1.ContourDeployment
		gateway            *gatewayv1alpha2.Gateway
		req                *reconcile.Request
		assertions         func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error)
	}{
		"A gateway for a reconcilable gatewayclass is reconciled": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Contour deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (not accepted) is not reconciled": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: gatewayv1alpha2.GatewayController(controller),
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionFalse,
							Reason: string(gatewayv1alpha2.GatewayClassReasonInvalidParameters),
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Scheduled: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deploy), deploy)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (non-matching controller) is not reconciled": {
			gatewayClass: &gatewayv1alpha2.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1alpha2.GatewayClassSpec{
					ControllerName: "someothercontroller.io/controller",
				},
				Status: gatewayv1alpha2.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1alpha2.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gatewayv1alpha2.GatewayClassReasonAccepted),
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Scheduled: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deploy), deploy)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"A gateway with no addresses results in an Envoy service with no loadBalancerIP": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "")
			},
		},
		"A gateway with one IP address results in an Envoy service with loadBalancerIP set to that IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.IPAddressType),
							Value: "172.18.255.207",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with two IP addresses results in an Envoy service with loadBalancerIP set to the first IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.IPAddressType),
							Value: "172.18.255.207",
						},
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.IPAddressType),
							Value: "172.18.255.999",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with one Hostname address results in an Envoy service with loadBalancerIP set to that hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.HostnameAddressType),
							Value: "projectcontour.io",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with two Hostname addresses results in an Envoy service with loadBalancerIP set to the first hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.HostnameAddressType),
							Value: "projectcontour.io",
						},
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.HostnameAddressType),
							Value: "anotherhost.io",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with one named address results in an Envoy service with no loadBalancerIP": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1alpha2.GatewayAddress{
						{
							Type:  gatewayapi.AddressTypePtr(gatewayv1alpha2.NamedAddressType),
							Value: "named-addresses-are-not-supported",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "")
			},
		},
		"Config from the Gateway's GatewayClass params is applied to the provisioned ContourConfiguration": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contourv1alpha1.ContourConfigurationSpec{
						EnableExternalNameService: pointer.Bool(true),
						Envoy: &contourv1alpha1.EnvoyConfig{
							Listener: &contourv1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: pointer.Bool(true),
							},
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contourv1alpha1.ContourConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contourv1alpha1.ContourConfigurationSpec{
					EnableExternalNameService: pointer.Bool(true),
					Gateway: &contourv1alpha1.GatewayConfig{
						GatewayRef: &contourv1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contourv1alpha1.EnvoyConfig{
						Listener: &contourv1alpha1.EnvoyListenerConfig{
							DisableMergeSlashes: pointer.Bool(true),
						},
						Service: &contourv1alpha1.NamespacedName{
							Namespace: gw.Namespace,
							Name:      "envoy-" + gw.Name,
						},
					},
				}

				assert.Equal(t, want, contourConfig.Spec)
			},
		},
		"Gateway-related config from the Gateway's GatewayClass params is overridden": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contourv1alpha1.ContourConfigurationSpec{
						Gateway: &contourv1alpha1.GatewayConfig{
							ControllerName: "some-controller",
							GatewayRef: &contourv1alpha1.NamespacedName{
								Namespace: "some-other-namespace",
								Name:      "some-other-gateway",
							},
						},
						Envoy: &contourv1alpha1.EnvoyConfig{
							Service: &contourv1alpha1.NamespacedName{
								Namespace: "some-other-namespace",
								Name:      "some-other-service",
							},
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contourv1alpha1.ContourConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contourv1alpha1.ContourConfigurationSpec{
					Gateway: &contourv1alpha1.GatewayConfig{
						GatewayRef: &contourv1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contourv1alpha1.EnvoyConfig{
						Service: &contourv1alpha1.NamespacedName{
							Namespace: gw.Namespace,
							Name:      "envoy-" + gw.Name,
						},
					},
				}

				assert.Equal(t, want, contourConfig.Spec)
			},
		},
		"If the Gateway's GatewayClass parametersRef is invalid it's ignored and the Gateway gets a default ContourConfiguration": {
			gatewayClass: reconcilableGatewayClassWithInvalidParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contourv1alpha1.ContourConfigurationSpec{
						EnableExternalNameService: pointer.Bool(true),
						Envoy: &contourv1alpha1.EnvoyConfig{
							Listener: &contourv1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: pointer.Bool(true),
							},
						},
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contourv1alpha1.ContourConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contourv1alpha1.ContourConfigurationSpec{
					Gateway: &contourv1alpha1.GatewayConfig{
						GatewayRef: &contourv1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contourv1alpha1.EnvoyConfig{
						Service: &contourv1alpha1.NamespacedName{
							Namespace: gw.Namespace,
							Name:      "envoy-" + gw.Name,
						},
					},
				}

				assert.Equal(t, want, contourConfig.Spec)
			},
		},
		"The Envoy service's ports are derived from the Gateway's listeners (http & https)": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Listeners: []gatewayv1alpha2.Listener{
						{
							Name:     "listener-1",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-2",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     82,
							Hostname: gatewayapi.ListenerHostname("foo.bar"),
						},
						// listener-3's port will be ignored because it's different than the previous HTTP listeners'
						{
							Name:     "listener-3",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     80,
						},
						// listener-4 will be ignored because it's an unsupported protocol
						{
							Name:     "listener-4",
							Protocol: gatewayv1alpha2.TCPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-5",
							Protocol: gatewayv1alpha2.HTTPSProtocolType,
							Port:     8443,
						},
						{
							Name:     "listener-6",
							Protocol: gatewayv1alpha2.TLSProtocolType,
							Port:     8443,
							Hostname: gatewayapi.ListenerHostname("foo.bar"),
						},
						// listener-7's port will be ignored because it's different than the previous HTTPS/TLS listeners'
						{
							Name:     "listener-7",
							Protocol: gatewayv1alpha2.HTTPSProtocolType,
							Port:     8444,
							Hostname: gatewayapi.ListenerHostname("foo.baz"),
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				// Get the expected Envoy service from the client.
				envoyService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: gw.Namespace,
						Name:      "envoy-" + gw.Name,
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(envoyService), envoyService))

				require.Len(t, envoyService.Spec.Ports, 2)
				assert.Contains(t, envoyService.Spec.Ports, corev1.ServicePort{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       82,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				})
				assert.Contains(t, envoyService.Spec.Ports, corev1.ServicePort{
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					Port:       8443,
					TargetPort: intstr.IntOrString{IntVal: 8443},
				})
			},
		},
		"The Envoy service's ports are derived from the Gateway's listeners (http only)": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Listeners: []gatewayv1alpha2.Listener{
						{
							Name:     "listener-1",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-2",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     82,
							Hostname: gatewayapi.ListenerHostname("foo.bar"),
						},
						// listener-3's port will be ignored because it's different than the previous HTTP listeners'
						{
							Name:     "listener-3",
							Protocol: gatewayv1alpha2.HTTPProtocolType,
							Port:     80,
						},
						// listener-4 will be ignored because it's an unsupported protocol
						{
							Name:     "listener-4",
							Protocol: gatewayv1alpha2.TCPProtocolType,
							Port:     82,
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				// Get the expected Envoy service from the client.
				envoyService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: gw.Namespace,
						Name:      "envoy-" + gw.Name,
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(envoyService), envoyService))

				require.Len(t, envoyService.Spec.Ports, 1)
				assert.Contains(t, envoyService.Spec.Ports, corev1.ServicePort{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       82,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				})
			},
		},
		"If ContourDeployment.Spec.Contour.Replicas is not specified, the Contour deployment defaults to 2 replicas": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				require.NotNil(t, deploy.Spec.Replicas)
				assert.EqualValues(t, 2, *deploy.Spec.Replicas)
			},
		},
		"If ContourDeployment.Spec.Contour.Replicas is specified, the Contour deployment gets that number of replicas": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Contour: &contourv1alpha1.ContourSettings{
						Replicas: 3,
					},
				},
			},
			gateway: &gatewayv1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1alpha2.GatewaySpec{
					GatewayClassName: gatewayv1alpha2.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1alpha2.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Scheduled: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				require.NotNil(t, deploy.Spec.Replicas)
				assert.EqualValues(t, 3, *deploy.Spec.Replicas)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme, err := provisioner.CreateScheme()
			require.NoError(t, err)

			client := fake.NewClientBuilder().WithScheme(scheme)
			if tc.gatewayClass != nil {
				client.WithObjects(tc.gatewayClass)
			}
			if tc.gatewayClassParams != nil {
				client.WithObjects(tc.gatewayClassParams)
			}
			if tc.gateway != nil {
				client.WithObjects(tc.gateway)
			}

			r := &gatewayReconciler{
				gatewayController: controller,
				client:            client.Build(),
				log:               logr.Discard(),
			}

			var req reconcile.Request
			if tc.req != nil {
				req = *tc.req
			} else {
				req = reconcile.Request{
					NamespacedName: keyFor(tc.gateway),
				}
			}

			_, err = r.Reconcile(context.Background(), req)

			tc.assertions(t, r, tc.gateway, err)
		})
	}
}

func assertEnvoyServiceLoadBalancerIP(t *testing.T, gateway *gatewayv1alpha2.Gateway, client client.Client, want string) {
	// Get the expected Envoy service from the client.
	envoyService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: gateway.Namespace,
			Name:      "envoy-" + gateway.Name,
		},
	}
	require.NoError(t, client.Get(context.Background(), keyFor(envoyService), envoyService))

	// Verify expected Spec.LoadBalancerIP.
	assert.Equal(t, want, envoyService.Spec.LoadBalancerIP)
}
