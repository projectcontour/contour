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
	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGatewayReconcile(t *testing.T) {
	const controller = "projectcontour.io/gateway-controller"

	reconcilableGatewayClass := func(name, controller string) *gatewayv1beta1.GatewayClass {
		return &gatewayv1beta1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: gatewayv1beta1.GatewayClassSpec{
				ControllerName: gatewayv1beta1.GatewayController(controller),
			},
			// the fake client lets us create resources with a status set
			Status: gatewayv1beta1.GatewayClassStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
						Status: metav1.ConditionTrue,
						Reason: string(gatewayv1beta1.GatewayClassReasonAccepted),
					},
				},
			},
		}
	}

	reconcilableGatewayClassWithParams := func(name, controller string) *gatewayv1beta1.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayv1beta1.ParametersReference{
			Group:     gatewayv1beta1.Group(contourv1alpha1.GroupVersion.Group),
			Kind:      "ContourDeployment",
			Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
			Name:      name + "-params",
		}
		return gc
	}

	reconcilableGatewayClassWithInvalidParams := func(name, controller string) *gatewayv1beta1.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayv1beta1.ParametersReference{
			Group:     gatewayv1beta1.Group(contourv1alpha1.GroupVersion.Group),
			Kind:      "InvalidKind",
			Namespace: ref.To(gatewayv1beta1.Namespace("projectcontour")),
			Name:      name + "-params",
		}
		return gc
	}

	tests := map[string]struct {
		gatewayClass       *gatewayv1beta1.GatewayClass
		gatewayClassParams *contourv1alpha1.ContourDeployment
		gateway            *gatewayv1beta1.Gateway
		req                *reconcile.Request
		assertions         func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error)
	}{
		"A gateway for a reconcilable gatewayclass is reconciled": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: gatewayv1beta1.GatewayController(controller),
				},
				Status: gatewayv1beta1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionFalse,
							Reason: string(gatewayv1beta1.GatewayClassReasonInvalidParameters),
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Accepted: true" condition set
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
			gatewayClass: &gatewayv1beta1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayv1beta1.GatewayClassSpec{
					ControllerName: "someothercontroller.io/controller",
				},
				Status: gatewayv1beta1.GatewayClassStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayv1beta1.GatewayClassConditionStatusAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(gatewayv1beta1.GatewayClassReasonAccepted),
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Accepted: true" condition set
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
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "")
			},
		},
		"A gateway with one IP address results in an Envoy service with loadBalancerIP set to that IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayv1beta1.IPAddressType),
							Value: "172.18.255.207",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with two IP addresses results in an Envoy service with loadBalancerIP set to the first IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayv1beta1.IPAddressType),
							Value: "172.18.255.207",
						},
						{
							Type:  ref.To(gatewayv1beta1.IPAddressType),
							Value: "172.18.255.999",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with one Hostname address results in an Envoy service with loadBalancerIP set to that hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayv1beta1.HostnameAddressType),
							Value: "projectcontour.io",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with two Hostname addresses results in an Envoy service with loadBalancerIP set to the first hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayv1beta1.HostnameAddressType),
							Value: "projectcontour.io",
						},
						{
							Type:  ref.To(gatewayv1beta1.HostnameAddressType),
							Value: "anotherhost.io",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with one custom address type results in an Envoy service with no loadBalancerIP": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Addresses: []gatewayv1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayv1beta1.AddressType("acme.io/CustomAddressType")),
							Value: "custom-address-types-are-not-supported",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
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
						EnableExternalNameService: ref.To(true),
						Envoy: &contourv1alpha1.EnvoyConfig{
							Listener: &contourv1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: ref.To(true),
							},
							Metrics: &contourv1alpha1.MetricsConfig{
								Port: 8003,
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
					EnableExternalNameService: ref.To(true),
					Gateway: &contourv1alpha1.GatewayConfig{
						GatewayRef: &contourv1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contourv1alpha1.EnvoyConfig{
						Listener: &contourv1alpha1.EnvoyListenerConfig{
							DisableMergeSlashes: ref.To(true),
						},
						Service: &contourv1alpha1.NamespacedName{
							Namespace: gw.Namespace,
							Name:      "envoy-" + gw.Name,
						},
						Metrics: &contourv1alpha1.MetricsConfig{
							Port: 8003,
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
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
						EnableExternalNameService: ref.To(true),
						Envoy: &contourv1alpha1.EnvoyConfig{
							Listener: &contourv1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: ref.To(true),
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Listeners: []gatewayv1beta1.Listener{
						{
							Name:     "listener-1",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-2",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     82,
							Hostname: ref.To(gatewayv1beta1.Hostname("foo.bar")),
						},
						// listener-3's port will be ignored because it's different than the previous HTTP listeners'
						{
							Name:     "listener-3",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     80,
						},
						// listener-4 will be ignored because it's an unsupported protocol
						{
							Name:     "listener-4",
							Protocol: gatewayv1beta1.TCPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-5",
							Protocol: gatewayv1beta1.HTTPSProtocolType,
							Port:     8443,
						},
						{
							Name:     "listener-6",
							Protocol: gatewayv1beta1.TLSProtocolType,
							Port:     8443,
							Hostname: ref.To(gatewayv1beta1.Hostname("foo.bar")),
						},
						// listener-7's port will be ignored because it's different than the previous HTTPS/TLS listeners'
						{
							Name:     "listener-7",
							Protocol: gatewayv1beta1.HTTPSProtocolType,
							Port:     8444,
							Hostname: ref.To(gatewayv1beta1.Hostname("foo.baz")),
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
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
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: "gatewayclass-1",
					Listeners: []gatewayv1beta1.Listener{
						{
							Name:     "listener-1",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     82,
						},
						{
							Name:     "listener-2",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     82,
							Hostname: ref.To(gatewayv1beta1.Hostname("foo.bar")),
						},
						// listener-3's port will be ignored because it's different than the previous HTTP listeners'
						{
							Name:     "listener-3",
							Protocol: gatewayv1beta1.HTTPProtocolType,
							Port:     80,
						},
						// listener-4 will be ignored because it's an unsupported protocol
						{
							Name:     "listener-4",
							Protocol: gatewayv1beta1.TCPProtocolType,
							Port:     82,
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
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
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
		"If ContourDeployment.Spec.Contour.Deployment is specified, the Contour deployment gets that settings": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Contour: &contourv1alpha1.ContourSettings{
						Replicas: 3,
						Deployment: &contourv1alpha1.DeploymentSettings{
							Replicas: 4,
							Strategy: &appsv1.DeploymentStrategy{
								Type: appsv1.RecreateDeploymentStrategyType,
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
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
				assert.EqualValues(t, 4, *deploy.Spec.Replicas)
				require.NotNil(t, deploy.Spec.Strategy)
				assert.EqualValues(t, appsv1.RecreateDeploymentStrategyType, deploy.Spec.Strategy.Type)
			},
		},
		"If ContourDeployment.Spec.Contour.NodePlacement is not specified, the Contour deployment has no node selector or tolerations set": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Contour: &contourv1alpha1.ContourSettings{},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				assert.Empty(t, deploy.Spec.Template.Spec.NodeSelector)
				assert.Empty(t, deploy.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Contour.NodePlacement is specified, it is used for the Contour deployment": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Contour: &contourv1alpha1.ContourSettings{
						NodePlacement: &contourv1alpha1.NodePlacement{
							NodeSelector: map[string]string{"foo": "bar"},
							Tolerations: []corev1.Toleration{
								{
									Key:      "toleration-key-1",
									Operator: corev1.TolerationOpEqual,
									Value:    "toleration-value-1",
								},
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				assert.Equal(t, map[string]string{"foo": "bar"}, deploy.Spec.Template.Spec.NodeSelector)
				assert.Equal(t, []corev1.Toleration{
					{
						Key:      "toleration-key-1",
						Operator: corev1.TolerationOpEqual,
						Value:    "toleration-value-1",
					},
				}, deploy.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NodePlacement is not specified, the Envoy workload has no node selector or tolerations set": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				daemonset := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(daemonset), daemonset))

				assert.Empty(t, daemonset.Spec.Template.Spec.NodeSelector)
				assert.Empty(t, daemonset.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NodePlacement is specified, it is used for the Envoy workload": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						NodePlacement: &contourv1alpha1.NodePlacement{
							NodeSelector: map[string]string{"foo": "bar"},
							Tolerations: []corev1.Toleration{
								{
									Key:      "toleration-key-1",
									Operator: corev1.TolerationOpEqual,
									Value:    "toleration-value-1",
								},
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				daemonset := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(daemonset), daemonset))

				assert.Equal(t, map[string]string{"foo": "bar"}, daemonset.Spec.Template.Spec.NodeSelector)
				assert.Equal(t, []corev1.Toleration{
					{
						Key:      "toleration-key-1",
						Operator: corev1.TolerationOpEqual,
						Value:    "toleration-value-1",
					},
				}, daemonset.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NetworkPublishing is not specified, the Envoy service defaults to a LoadBalancer with no annotations": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(svc), svc))

				assert.Equal(t, corev1.ServiceTypeLoadBalancer, svc.Spec.Type)
				assert.Empty(t, svc.Annotations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NetworkPublishing is specified, its settings are used for the Envoy service": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						NetworkPublishing: &contourv1alpha1.NetworkPublishing{
							Type:                  contourv1alpha1.NodePortServicePublishingType,
							ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
							ServiceAnnotations: map[string]string{
								"key-1": "val-1",
								"key-2": "val-2",
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
					Listeners: []gatewayv1beta1.Listener{
						{
							Protocol: gatewayv1beta1.HTTPProtocolType,
							AllowedRoutes: &gatewayv1beta1.AllowedRoutes{
								Namespaces: &gatewayv1beta1.RouteNamespaces{
									From: ref.To(gatewayv1beta1.NamespacesFromAll),
								},
							},
							Name: gatewayv1beta1.SectionName("http"),
							Port: gatewayv1beta1.PortNumber(30000),
						},
						{
							Name:     gatewayv1beta1.SectionName("https"),
							Port:     gatewayv1beta1.PortNumber(30001),
							Protocol: gatewayv1beta1.HTTPSProtocolType,
							AllowedRoutes: &gatewayv1beta1.AllowedRoutes{
								Namespaces: &gatewayv1beta1.RouteNamespaces{
									From: ref.To(gatewayv1beta1.NamespacesFromAll),
								},
							},
							TLS: &gatewayv1beta1.GatewayTLSConfig{
								Mode: ref.To(gatewayv1beta1.TLSModeTerminate),
							},
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(svc), svc))
				assert.Equal(t, corev1.ServiceExternalTrafficPolicyTypeCluster, svc.Spec.ExternalTrafficPolicy)
				assert.Equal(t, corev1.ServiceTypeNodePort, svc.Spec.Type)
				require.Len(t, svc.Annotations, 2)
				assert.Equal(t, "val-1", svc.Annotations["key-1"])
				assert.Equal(t, "val-2", svc.Annotations["key-2"])

				assert.Len(t, svc.Spec.Ports, 2)
				assert.Equal(t, int32(30000), svc.Spec.Ports[0].NodePort)
				assert.Equal(t, int32(80), svc.Spec.Ports[0].Port)
				assert.Equal(t, int32(30001), svc.Spec.Ports[1].NodePort)
				assert.Equal(t, int32(443), svc.Spec.Ports[1].Port)
			},
		},
		"If ContourDeployment.Spec.Envoy.WorkloadType is set to Deployment, an Envoy deployment is provisioned with the specified number of replicas": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						WorkloadType: contourv1alpha1.WorkloadTypeDeployment,
						Replicas:     7,
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
				assert.EqualValues(t, 7, *deploy.Spec.Replicas)

				// Verify that a daemonset has *not* been created
				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(ds), ds)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"If ContourDeployment.Spec.Envoy.WorkloadType is set to Deployment," +
			"an Envoy deployment is provisioned with the settings come from DeployemntSettings": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						WorkloadType: contourv1alpha1.WorkloadTypeDeployment,
						Replicas:     7,
						Deployment: &contourv1alpha1.DeploymentSettings{
							Replicas: 6,
							Strategy: &appsv1.DeploymentStrategy{
								Type: appsv1.RecreateDeploymentStrategyType,
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has an "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				assert.NotNil(t, deploy.Spec.Replicas)
				assert.EqualValues(t, 6, *deploy.Spec.Replicas)

				assert.NotNil(t, deploy.Spec.Strategy)
				assert.EqualValues(t, appsv1.RecreateDeploymentStrategyType, deploy.Spec.Strategy.Type)

				// Verify that a daemonset has *not* been created
				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(ds), ds)
				assert.True(t, errors.IsNotFound(err))
			},
		},

		"If ContourDeployment.Spec.Envoy.PodAnnotations is specified, the Envoy pods' have annotations for prometheus & user-defined": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						PodAnnotations: map[string]string{
							"key": "val",
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.Contains(t, ds.Spec.Template.ObjectMeta.Annotations, "key")
			},
		},

		"If ContourDeployment.Spec.Envoy.WorkloadType is set to DaemonSet," +
			"an Envoy daemonset is provisioned with the strategy that come from DaemonsetSettings": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contourv1alpha1.ContourDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contourv1alpha1.ContourDeploymentSpec{
					Envoy: &contourv1alpha1.EnvoySettings{
						WorkloadType: contourv1alpha1.WorkloadTypeDaemonSet,
						DaemonSet: &contourv1alpha1.DaemonSetSettings{
							UpdateStrategy: &appsv1.DaemonSetUpdateStrategy{
								Type: appsv1.OnDeleteDaemonSetStrategyType,
							},
						},
					},
				},
			},
			gateway: &gatewayv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayv1beta1.GatewaySpec{
					GatewayClassName: gatewayv1beta1.ObjectName("gatewayclass-1"),
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayv1beta1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has an "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayv1beta1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.EqualValues(t, appsv1.OnDeleteDaemonSetStrategyType, ds.Spec.UpdateStrategy.Type)

				// Verify that a deployment has *not* been created
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deployment), deployment)
				assert.True(t, errors.IsNotFound(err))
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

func assertEnvoyServiceLoadBalancerIP(t *testing.T, gateway *gatewayv1beta1.Gateway, client client.Client, want string) {
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
