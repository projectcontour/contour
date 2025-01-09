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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/provisioner/model"
)

func TestGatewayReconcile(t *testing.T) {
	const controller = "projectcontour.io/gateway-controller"

	reconcilableGatewayClass := func(name, controller string) *gatewayapi_v1.GatewayClass {
		return &gatewayapi_v1.GatewayClass{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: name,
			},
			Spec: gatewayapi_v1.GatewayClassSpec{
				ControllerName: gatewayapi_v1.GatewayController(controller),
			},
			// the fake client lets us create resources with a status set
			Status: gatewayapi_v1.GatewayClassStatus{
				Conditions: []meta_v1.Condition{
					{
						Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
						Status: meta_v1.ConditionTrue,
						Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
					},
				},
			},
		}
	}

	reconcilableGatewayClassWithParams := func(name, controller string) *gatewayapi_v1.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayapi_v1.ParametersReference{
			Group:     gatewayapi_v1.Group(contour_v1alpha1.GroupVersion.Group),
			Kind:      "ContourDeployment",
			Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
			Name:      name + "-params",
		}
		return gc
	}

	reconcilableGatewayClassWithInvalidParams := func(name, controller string) *gatewayapi_v1.GatewayClass {
		gc := reconcilableGatewayClass(name, controller)
		gc.Spec.ParametersRef = &gatewayapi_v1.ParametersReference{
			Group:     gatewayapi_v1.Group(contour_v1alpha1.GroupVersion.Group),
			Kind:      "InvalidKind",
			Namespace: ptr.To(gatewayapi_v1.Namespace("projectcontour")),
			Name:      name + "-params",
		}
		return gc
	}

	makeGateway := func() *gatewayapi_v1.Gateway {
		return &gatewayapi_v1.Gateway{
			ObjectMeta: meta_v1.ObjectMeta{
				Namespace: "gateway-1",
				Name:      "gateway-1",
			},
			Spec: gatewayapi_v1.GatewaySpec{
				GatewayClassName: gatewayapi_v1.ObjectName("gatewayclass-1"),
			},
		}
	}

	makeGatewayWithAddrs := func(addrs []gatewayapi_v1.GatewayAddress) *gatewayapi_v1.Gateway {
		gtw := makeGateway()
		gtw.Spec.Addresses = addrs
		return gtw
	}

	makeGatewayWithListeners := func(listeners []gatewayapi_v1.Listener) *gatewayapi_v1.Gateway {
		gtw := makeGateway()
		gtw.Spec.Listeners = listeners
		return gtw
	}

	tests := map[string]struct {
		gatewayClass       *gatewayapi_v1.GatewayClass
		gatewayClassParams *contour_v1alpha1.ContourDeployment
		gateway            *gatewayapi_v1.Gateway
		req                *reconcile.Request
		assertions         func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error)
	}{
		"A gateway for a reconcilable gatewayclass is reconciled": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway:      makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Contour deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (not accepted) is not reconciled": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: gatewayapi_v1.GatewayController(controller),
				},
				Status: gatewayapi_v1.GatewayClassStatus{
					Conditions: []meta_v1.Condition{
						{
							Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
							Status: meta_v1.ConditionFalse,
							Reason: string(gatewayapi_v1.GatewayClassReasonInvalidParameters),
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Accepted: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deploy), deploy)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"A gateway for a non-reconcilable gatewayclass (non-matching controller) is not reconciled": {
			gatewayClass: &gatewayapi_v1.GatewayClass{
				ObjectMeta: meta_v1.ObjectMeta{
					Name: "gatewayclass-1",
				},
				Spec: gatewayapi_v1.GatewayClassSpec{
					ControllerName: "someothercontroller.io/controller",
				},
				Status: gatewayapi_v1.GatewayClassStatus{
					Conditions: []meta_v1.Condition{
						{
							Type:   string(gatewayapi_v1.GatewayClassConditionStatusAccepted),
							Status: meta_v1.ConditionTrue,
							Reason: string(gatewayapi_v1.GatewayClassReasonAccepted),
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify that the Gateway has not had a "Accepted: true" condition set
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Empty(t, gw.Status.Conditions, 0)

				// Verify the Contour deployment has not been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gateway:      makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "")
			},
		},
		"A gateway with one IP address results in an Envoy service with loadBalancerIP set to that IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithAddrs([]gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.IPAddressType),
					Value: "172.18.255.207",
				},
			}),

			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with two IP addresses results in an Envoy service with loadBalancerIP set to the first IP address": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithAddrs([]gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.IPAddressType),
					Value: "172.18.255.207",
				},
				{
					Type:  ptr.To(gatewayapi_v1.IPAddressType),
					Value: "172.18.255.999",
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "172.18.255.207")
			},
		},
		"A gateway with one Hostname address results in an Envoy service with loadBalancerIP set to that hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithAddrs([]gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.HostnameAddressType),
					Value: "projectcontour.io",
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with two Hostname addresses results in an Envoy service with loadBalancerIP set to the first hostname": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithAddrs([]gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.HostnameAddressType),
					Value: "projectcontour.io",
				},
				{
					Type:  ptr.To(gatewayapi_v1.HostnameAddressType),
					Value: "anotherhost.io",
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "projectcontour.io")
			},
		},
		"A gateway with one custom address type results in an Envoy service with no loadBalancerIP": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithAddrs([]gatewayapi_v1.GatewayAddress{
				{
					Type:  ptr.To(gatewayapi_v1.AddressType("acme.io/CustomAddressType")),
					Value: "custom-address-types-are-not-supported",
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				assertEnvoyServiceLoadBalancerIP(t, gw, r.client, "")
			},
		},
		"Config from the Gateway's GatewayClass params is applied to the provisioned ContourConfiguration": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contour_v1alpha1.ContourConfigurationSpec{
						EnableExternalNameService: ptr.To(true),
						Envoy: &contour_v1alpha1.EnvoyConfig{
							Listener: &contour_v1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: ptr.To(true),
							},
							Metrics: &contour_v1alpha1.MetricsConfig{
								Port: 8003,
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contour_v1alpha1.ContourConfiguration{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contour_v1alpha1.ContourConfigurationSpec{
					EnableExternalNameService: ptr.To(true),
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Listener: &contour_v1alpha1.EnvoyListenerConfig{
							DisableMergeSlashes: ptr.To(true),
						},
						Service: &contour_v1alpha1.NamespacedName{
							Namespace: gw.Namespace,
							Name:      "envoy-" + gw.Name,
						},
						Metrics: &contour_v1alpha1.MetricsConfig{
							Port: 8003,
						},
					},
				}

				assert.Equal(t, want, contourConfig.Spec)
			},
		},
		"Gateway-related config from the Gateway's GatewayClass params is overridden": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contour_v1alpha1.ContourConfigurationSpec{
						Gateway: &contour_v1alpha1.GatewayConfig{
							GatewayRef: contour_v1alpha1.NamespacedName{
								Namespace: "some-other-namespace",
								Name:      "some-other-gateway",
							},
						},
						Envoy: &contour_v1alpha1.EnvoyConfig{
							Service: &contour_v1alpha1.NamespacedName{
								Namespace: "some-other-namespace",
								Name:      "some-other-service",
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contour_v1alpha1.ContourConfiguration{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contour_v1alpha1.ContourConfigurationSpec{
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Service: &contour_v1alpha1.NamespacedName{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					RuntimeSettings: &contour_v1alpha1.ContourConfigurationSpec{
						EnableExternalNameService: ptr.To(true),
						Envoy: &contour_v1alpha1.EnvoyConfig{
							Listener: &contour_v1alpha1.EnvoyListenerConfig{
								DisableMergeSlashes: ptr.To(true),
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the ContourConfiguration has been created
				contourConfig := &contour_v1alpha1.ContourConfiguration{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contourconfig-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(contourConfig), contourConfig))

				want := contour_v1alpha1.ContourConfigurationSpec{
					Gateway: &contour_v1alpha1.GatewayConfig{
						GatewayRef: contour_v1alpha1.NamespacedName{
							Namespace: gw.Name,
							Name:      gw.Name,
						},
					},
					Envoy: &contour_v1alpha1.EnvoyConfig{
						Service: &contour_v1alpha1.NamespacedName{
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
			gateway: makeGatewayWithListeners([]gatewayapi_v1.Listener{
				{
					Name:     "listener-1",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     80,
				},
				{
					Name:     "listener-2",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gatewayapi_v1.Hostname("foo.bar")),
				},
				{
					Name:     "listener-3",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     81,
				},
				// listener-4 will be ignored because it's an unsupported protocol
				{
					Name:     "listener-4",
					Protocol: gatewayapi_v1.UDPProtocolType,
					Port:     82,
				},
				{
					Name:     "listener-5",
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					Port:     443,
				},
				{
					Name:     "listener-6",
					Protocol: gatewayapi_v1.TLSProtocolType,
					Port:     443,
					Hostname: ptr.To(gatewayapi_v1.Hostname("foo.bar")),
				},
				{
					Name:     "listener-7",
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					Port:     8443,
					Hostname: ptr.To(gatewayapi_v1.Hostname("foo.baz")),
				},
			}),

			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				// Get the expected Envoy service from the client.
				envoyService := &core_v1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: gw.Namespace,
						Name:      "envoy-" + gw.Name,
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(envoyService), envoyService))

				require.Len(t, envoyService.Spec.Ports, 4)
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "http-80",
					Protocol:   core_v1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				})
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "http-81",
					Protocol:   core_v1.ProtocolTCP,
					Port:       81,
					TargetPort: intstr.IntOrString{IntVal: 8081},
				})
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "https-443",
					Protocol:   core_v1.ProtocolTCP,
					Port:       443,
					TargetPort: intstr.IntOrString{IntVal: 8443},
				})
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "https-8443",
					Protocol:   core_v1.ProtocolTCP,
					Port:       8443,
					TargetPort: intstr.IntOrString{IntVal: 16443},
				})
			},
		},
		"The Envoy service's ports are derived from the Gateway's listeners (http only)": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: makeGatewayWithListeners([]gatewayapi_v1.Listener{
				{
					Name:     "listener-1",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     80,
				},
				{
					Name:     "listener-2",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     80,
					Hostname: ptr.To(gatewayapi_v1.Hostname("foo.bar")),
				},
				{
					Name:     "listener-3",
					Protocol: gatewayapi_v1.HTTPProtocolType,
					Port:     8080,
				},
				// listener-4 will be ignored because it's an unsupported protocol
				{
					Name:     "listener-4",
					Protocol: gatewayapi_v1.UDPProtocolType,
					Port:     82,
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)
				// Get the expected Envoy service from the client.
				envoyService := &core_v1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: gw.Namespace,
						Name:      "envoy-" + gw.Name,
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(envoyService), envoyService))

				require.Len(t, envoyService.Spec.Ports, 2)
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "http-80",
					Protocol:   core_v1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				})
				assert.Contains(t, envoyService.Spec.Ports, core_v1.ServicePort{
					Name:       "http-8080",
					Protocol:   core_v1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.IntOrString{IntVal: 16080},
				})
			},
		},
		"If ContourDeployment.Spec.Contour.Replicas is not specified, the Contour deployment defaults to 2 replicas": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Contour: &contour_v1alpha1.ContourSettings{
						Replicas: 3,
						Deployment: &contour_v1alpha1.DeploymentSettings{
							Replicas: 4,
							Strategy: &apps_v1.DeploymentStrategy{
								Type: apps_v1.RecreateDeploymentStrategyType,
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the Deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				require.NotNil(t, deploy.Spec.Replicas)
				assert.EqualValues(t, 4, *deploy.Spec.Replicas)
				require.NotNil(t, deploy.Spec.Strategy)
				assert.EqualValues(t, apps_v1.RecreateDeploymentStrategyType, deploy.Spec.Strategy.Type)
			},
		},
		"If ContourDeployment.Spec.Contour.NodePlacement is not specified, the Contour deployment has no node selector or tolerations set": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Contour: &contour_v1alpha1.ContourSettings{},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Contour: &contour_v1alpha1.ContourSettings{
						NodePlacement: &contour_v1alpha1.NodePlacement{
							NodeSelector: map[string]string{"foo": "bar"},
							Tolerations: []core_v1.Toleration{
								{
									Key:      "toleration-key-1",
									Operator: core_v1.TolerationOpEqual,
									Value:    "toleration-value-1",
								},
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				assert.Equal(t, map[string]string{"foo": "bar"}, deploy.Spec.Template.Spec.NodeSelector)
				assert.Equal(t, []core_v1.Toleration{
					{
						Key:      "toleration-key-1",
						Operator: core_v1.TolerationOpEqual,
						Value:    "toleration-value-1",
					},
				}, deploy.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NodePlacement is not specified, the Envoy workload has no node selector or tolerations set": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				daemonset := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						NodePlacement: &contour_v1alpha1.NodePlacement{
							NodeSelector: map[string]string{"foo": "bar"},
							Tolerations: []core_v1.Toleration{
								{
									Key:      "toleration-key-1",
									Operator: core_v1.TolerationOpEqual,
									Value:    "toleration-value-1",
								},
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				daemonset := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(daemonset), daemonset))

				assert.Equal(t, map[string]string{"foo": "bar"}, daemonset.Spec.Template.Spec.NodeSelector)
				assert.Equal(t, []core_v1.Toleration{
					{
						Key:      "toleration-key-1",
						Operator: core_v1.TolerationOpEqual,
						Value:    "toleration-value-1",
					},
				}, daemonset.Spec.Template.Spec.Tolerations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NetworkPublishing is not specified, the Envoy service defaults to a LoadBalancer with no annotations": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				svc := &core_v1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(svc), svc))

				assert.Equal(t, core_v1.ServiceTypeLoadBalancer, svc.Spec.Type)
				assert.Empty(t, svc.Annotations)
			},
		},
		"If ContourDeployment.Spec.Envoy.NetworkPublishing is specified, its settings are used for the Envoy service": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						NetworkPublishing: &contour_v1alpha1.NetworkPublishing{
							Type:                  contour_v1alpha1.NodePortServicePublishingType,
							ExternalTrafficPolicy: core_v1.ServiceExternalTrafficPolicyTypeCluster,
							IPFamilyPolicy:        core_v1.IPFamilyPolicyPreferDualStack,
							ServiceAnnotations: map[string]string{
								"key-1": "val-1",
								"key-2": "val-2",
							},
						},
					},
				},
			},
			gateway: makeGatewayWithListeners([]gatewayapi_v1.Listener{
				{
					Protocol: gatewayapi_v1.HTTPProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					Name: gatewayapi_v1.SectionName("http"),
					Port: gatewayapi_v1.PortNumber(30000),
				},
				{
					Name:     gatewayapi_v1.SectionName("https"),
					Port:     gatewayapi_v1.PortNumber(30001),
					Protocol: gatewayapi_v1.HTTPSProtocolType,
					AllowedRoutes: &gatewayapi_v1.AllowedRoutes{
						Namespaces: &gatewayapi_v1.RouteNamespaces{
							From: ptr.To(gatewayapi_v1.NamespacesFromAll),
						},
					},
					TLS: &gatewayapi_v1.GatewayTLSConfig{
						Mode: ptr.To(gatewayapi_v1.TLSModeTerminate),
					},
				},
			}),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				svc := &core_v1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(svc), svc))
				assert.Equal(t, core_v1.ServiceExternalTrafficPolicyTypeCluster, svc.Spec.ExternalTrafficPolicy)
				assert.Equal(t, ptr.To(core_v1.IPFamilyPolicyPreferDualStack), svc.Spec.IPFamilyPolicy)
				assert.Equal(t, core_v1.ServiceTypeNodePort, svc.Spec.Type)
				require.Len(t, svc.Annotations, 2)
				assert.Equal(t, "val-1", svc.Annotations["key-1"])
				assert.Equal(t, "val-2", svc.Annotations["key-2"])

				assert.Len(t, svc.Spec.Ports, 2)
				assert.Equal(t, int32(30000), svc.Spec.Ports[0].NodePort)
				assert.Equal(t, int32(30000), svc.Spec.Ports[0].Port)
				assert.Equal(t, int32(30001), svc.Spec.Ports[1].NodePort)
				assert.Equal(t, int32(30001), svc.Spec.Ports[1].Port)
			},
		},
		"If ContourDeployment.Spec.Envoy.WorkloadType is set to Deployment, an Envoy deployment is provisioned with the specified number of replicas": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						WorkloadType: contour_v1alpha1.WorkloadTypeDeployment,
						Replicas:     7,
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
				assert.EqualValues(t, 7, *deploy.Spec.Replicas)

				// Verify that a daemonset has *not* been created
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						WorkloadType: contour_v1alpha1.WorkloadTypeDeployment,
						Replicas:     7,
						Deployment: &contour_v1alpha1.DeploymentSettings{
							Replicas: 6,
							Strategy: &apps_v1.DeploymentStrategy{
								Type: apps_v1.RecreateDeploymentStrategyType,
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has an "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))

				assert.NotNil(t, deploy.Spec.Replicas)
				assert.EqualValues(t, 6, *deploy.Spec.Replicas)

				assert.NotNil(t, deploy.Spec.Strategy)
				assert.EqualValues(t, apps_v1.RecreateDeploymentStrategyType, deploy.Spec.Strategy.Type)

				// Verify that a daemonset has *not* been created
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
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
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						PodAnnotations: map[string]string{
							"key": "val",
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the service has been created
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.Contains(t, ds.Spec.Template.ObjectMeta.Annotations, "key")
			},
		},

		"If ContourDeployment.Spec.Envoy.BaseID is specified, the Envoy container's arguments contain --base-id": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						BaseID: 1,
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, _ *gatewayapi_v1.Gateway, _ error) {
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.Contains(t, ds.Spec.Template.Spec.Containers[1].Args, "--base-id 1")
			},
		},

		"If ContourDeployment.Spec.Envoy.OverloadMaxHeapSize is specified, the envoy-initconfig container's arguments contain --overload-max-heap": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						OverloadMaxHeapSize: 10000000,
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, _ *gatewayapi_v1.Gateway, _ error) {
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.Contains(t, ds.Spec.Template.Spec.InitContainers[0].Args, "--overload-max-heap=10000000")
			},
		},

		"If ContourDeployment.Spec.Envoy.OverloadMaxHeapSize is not specified, the envoy-initconfig container's arguments contain --overload-max-heap=0": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, _ *gatewayapi_v1.Gateway, _ error) {
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.Contains(t, ds.Spec.Template.Spec.InitContainers[0].Args, "--overload-max-heap=0")
			},
		},

		"If ContourDeployment.Spec.Contour.PodAnnotations is specified, the Contour pods' have annotations for prometheus & user-defined": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Contour: &contour_v1alpha1.ContourSettings{
						PodAnnotations: map[string]string{
							"key": "val",
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has a "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the deployment has been created
				deploy := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "contour-gateway-1",
					},
				}

				require.NoError(t, r.client.Get(context.Background(), keyFor(deploy), deploy))
				assert.Contains(t, deploy.Spec.Template.ObjectMeta.Annotations, "key")
			},
		},

		"If ContourDeployment.Spec.Envoy.WorkloadType is set to DaemonSet," +
			"an Envoy daemonset is provisioned with the strategy that come from DaemonsetSettings": {
			gatewayClass: reconcilableGatewayClassWithParams("gatewayclass-1", controller),
			gatewayClassParams: &contour_v1alpha1.ContourDeployment{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "gatewayclass-1-params",
				},
				Spec: contour_v1alpha1.ContourDeploymentSpec{
					Envoy: &contour_v1alpha1.EnvoySettings{
						WorkloadType: contour_v1alpha1.WorkloadTypeDaemonSet,
						DaemonSet: &contour_v1alpha1.DaemonSetSettings{
							UpdateStrategy: &apps_v1.DaemonSetUpdateStrategy{
								Type: apps_v1.OnDeleteDaemonSetStrategyType,
							},
						},
					},
				},
			},
			gateway: makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				// Verify the Gateway has an "Accepted: true" condition
				require.NoError(t, r.client.Get(context.Background(), keyFor(gw), gw))
				require.Len(t, gw.Status.Conditions, 1)
				assert.Equal(t, string(gatewayapi_v1.GatewayConditionAccepted), gw.Status.Conditions[0].Type)
				assert.Equal(t, meta_v1.ConditionTrue, gw.Status.Conditions[0].Status)

				// Verify the daemonset has been created
				ds := &apps_v1.DaemonSet{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				require.NoError(t, r.client.Get(context.Background(), keyFor(ds), ds))
				assert.EqualValues(t, apps_v1.OnDeleteDaemonSetStrategyType, ds.Spec.UpdateStrategy.Type)

				// Verify that a deployment has *not* been created
				deployment := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: "gateway-1",
						Name:      "envoy-gateway-1",
					},
				}
				err := r.client.Get(context.Background(), keyFor(deployment), deployment)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		"The Gateway's infrastructure labels and annotations are set on all resources": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway: &gatewayapi_v1.Gateway{
				ObjectMeta: meta_v1.ObjectMeta{
					Namespace: "gateway-1",
					Name:      "gateway-1",
				},
				Spec: gatewayapi_v1.GatewaySpec{
					GatewayClassName: gatewayapi_v1.ObjectName("gatewayclass-1"),
					Infrastructure: &gatewayapi_v1.GatewayInfrastructure{
						Labels: map[gatewayapi_v1.LabelKey]gatewayapi_v1.LabelValue{
							"projectcontour.io/label-1": "label-value-1",
							"projectcontour.io/label-2": "label-value-2",
						},
						Annotations: map[gatewayapi_v1.AnnotationKey]gatewayapi_v1.AnnotationValue{
							"projectcontour.io/annotation-1": "annotation-value-1",
							"projectcontour.io/annotation-2": "annotation-value-2",
						},
					},
				},
			},
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				for _, obj := range []client.Object{
					&apps_v1.Deployment{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&apps_v1.DaemonSet{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&core_v1.Service{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&core_v1.Service{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&contour_v1alpha1.ContourConfiguration{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contourconfig-gateway-1"},
					},
					&core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contourcert-gateway-1"},
					},
					&core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoycert-gateway-1"},
					},
					&core_v1.ServiceAccount{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&core_v1.ServiceAccount{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&rbac_v1.ClusterRole{
						ObjectMeta: meta_v1.ObjectMeta{Name: "contour-gateway-1-gateway-1"},
					},
					&rbac_v1.ClusterRoleBinding{
						ObjectMeta: meta_v1.ObjectMeta{Name: "contour-gateway-1-gateway-1"},
					},
					&rbac_v1.Role{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&rbac_v1.RoleBinding{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-rolebinding-gateway-1"},
					},
				} {
					require.NoError(t, r.client.Get(context.Background(), keyFor(obj), obj))

					for k, v := range gw.Spec.Infrastructure.Labels {
						assert.Equal(t, obj.GetLabels()[string(k)], string(v))
					}
					for k, v := range gw.Spec.Infrastructure.Annotations {
						assert.Equal(t, obj.GetAnnotations()[string(k)], string(v))
					}
				}
			},
		},
		"Gateway owner labels are set on all resources": {
			gatewayClass: reconcilableGatewayClass("gatewayclass-1", controller),
			gateway:      makeGateway(),
			assertions: func(t *testing.T, r *gatewayReconciler, gw *gatewayapi_v1.Gateway, reconcileErr error) {
				require.NoError(t, reconcileErr)

				for _, obj := range []client.Object{
					&apps_v1.Deployment{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&apps_v1.DaemonSet{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&core_v1.Service{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&core_v1.Service{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&contour_v1alpha1.ContourConfiguration{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contourconfig-gateway-1"},
					},
					&core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contourcert-gateway-1"},
					},
					&core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoycert-gateway-1"},
					},
					&core_v1.ServiceAccount{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&core_v1.ServiceAccount{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "envoy-gateway-1"},
					},
					&rbac_v1.ClusterRole{
						ObjectMeta: meta_v1.ObjectMeta{Name: "contour-gateway-1-gateway-1"},
					},
					&rbac_v1.ClusterRoleBinding{
						ObjectMeta: meta_v1.ObjectMeta{Name: "contour-gateway-1-gateway-1"},
					},
					&rbac_v1.Role{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-gateway-1"},
					},
					&rbac_v1.RoleBinding{
						ObjectMeta: meta_v1.ObjectMeta{Namespace: "gateway-1", Name: "contour-rolebinding-gateway-1"},
					},
				} {
					require.NoError(t, r.client.Get(context.Background(), keyFor(obj), obj))

					assert.Equal(t, gw.Name, obj.GetLabels()[model.ContourOwningGatewayNameLabel])
					assert.Equal(t, gw.Name, obj.GetLabels()[model.GatewayAPIOwningGatewayNameLabel])
				}
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
				client.WithStatusSubresource(tc.gatewayClass)
			}
			if tc.gatewayClassParams != nil {
				client.WithObjects(tc.gatewayClassParams)
			}
			if tc.gateway != nil {
				client.WithObjects(tc.gateway)
				client.WithStatusSubresource(tc.gateway)
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

func assertEnvoyServiceLoadBalancerIP(t *testing.T, gateway *gatewayapi_v1.Gateway, client client.Client, want string) {
	// Get the expected Envoy service from the client.
	envoyService := &core_v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Namespace: gateway.Namespace,
			Name:      "envoy-" + gateway.Name,
		},
	}
	require.NoError(t, client.Get(context.Background(), keyFor(envoyService), envoyService))

	// Verify expected Spec.LoadBalancerIP.
	assert.Equal(t, want, envoyService.Spec.LoadBalancerIP)
}
