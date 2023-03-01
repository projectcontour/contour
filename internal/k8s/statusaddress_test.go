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

package k8s

import (
	"testing"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/projectcontour/contour/internal/ingressclass"
	"github.com/projectcontour/contour/internal/k8s/mocks"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestServiceStatusLoadBalancerWatcherOnAdd(t *testing.T) {
	lbstatus := make(chan v1.LoadBalancerStatus, 1)
	sw := ServiceStatusLoadBalancerWatcher{
		ServiceName: "envoy",
		LBStatus:    lbstatus,
		Log:         fixture.NewTestLogger(t),
	}

	recv := func() (v1.LoadBalancerStatus, bool) {
		select {
		case lbs := <-sw.LBStatus:
			return lbs, true
		default:
			return v1.LoadBalancerStatus{}, false
		}
	}

	// assert adding something other than a service generates no notification.
	sw.OnAdd(&v1.Pod{})
	_, ok := recv()
	if ok {
		t.Fatalf("expected no result when adding")
	}

	// assert adding a service with an different name generates no notification
	var svc v1.Service
	svc.Name = "potato"
	sw.OnAdd(&svc)
	_, ok = recv()
	if ok {
		t.Fatalf("expected no result when adding a service with a different name")
	}

	// assert adding a service with the correct name generates a notification
	svc.Name = sw.ServiceName
	svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{Hostname: "projectcontour.io"}}
	sw.OnAdd(&svc)
	got, ok := recv()
	if !ok {
		t.Fatalf("expected result when adding a service with the correct name")
	}
	want := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{Hostname: "projectcontour.io"}},
	}
	assert.Equal(t, got, want)
}

func TestServiceStatusLoadBalancerWatcherOnUpdate(t *testing.T) {
	lbstatus := make(chan v1.LoadBalancerStatus, 1)

	sw := ServiceStatusLoadBalancerWatcher{
		ServiceName: "envoy",
		LBStatus:    lbstatus,
		Log:         fixture.NewTestLogger(t),
	}

	recv := func() (v1.LoadBalancerStatus, bool) {
		select {
		case lbs := <-sw.LBStatus:
			return lbs, true
		default:
			return v1.LoadBalancerStatus{}, false
		}
	}

	// assert updating something other than a service generates no notification.
	sw.OnUpdate(&v1.Pod{}, &v1.Pod{})
	_, ok := recv()
	if ok {
		t.Fatalf("expected no result when updating")
	}

	// assert updating a service with an different name generates no notification
	var oldSvc, newSvc v1.Service
	oldSvc.Name = "potato"
	newSvc.Name = "elephant"
	sw.OnUpdate(&oldSvc, &newSvc)
	_, ok = recv()
	if ok {
		t.Fatalf("expected no result when updating a service with a different name")
	}

	// assert updating a service with the correct name generates a notification
	var svc v1.Service
	svc.Name = sw.ServiceName
	svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{Hostname: "projectcontour.io"}}
	sw.OnUpdate(&oldSvc, &svc)
	got, ok := recv()
	if !ok {
		t.Fatalf("expected result when updating a service with the correct name")
	}
	want := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{Hostname: "projectcontour.io"}},
	}
	assert.Equal(t, got, want)
}

func TestServiceStatusLoadBalancerWatcherOnDelete(t *testing.T) {
	lbstatus := make(chan v1.LoadBalancerStatus, 1)

	sw := ServiceStatusLoadBalancerWatcher{
		ServiceName: "envoy",
		LBStatus:    lbstatus,
		Log:         fixture.NewTestLogger(t),
	}

	recv := func() (v1.LoadBalancerStatus, bool) {
		select {
		case lbs := <-sw.LBStatus:
			return lbs, true
		default:
			return v1.LoadBalancerStatus{}, false
		}
	}

	// assert deleting something other than a service generates no notification.
	sw.OnDelete(&v1.Pod{})
	_, ok := recv()
	if ok {
		t.Fatalf("expected no result when deleting")
	}

	// assert adding a service with an different name generates no notification
	var svc v1.Service
	svc.Name = "potato"
	sw.OnDelete(&svc)
	_, ok = recv()
	if ok {
		t.Fatalf("expected no result when deleting a service with a different name")
	}

	// assert deleting a service with the correct name generates a blank notification
	svc.Name = sw.ServiceName
	svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{Hostname: "projectcontour.io"}}
	sw.OnDelete(&svc)
	got, ok := recv()
	if !ok {
		t.Fatalf("expected result when deleting a service with the correct name")
	}
	want := v1.LoadBalancerStatus{
		Ingress: nil,
	}
	assert.Equal(t, got, want)
}

func TestStatusAddressUpdater(t *testing.T) {
	const objName = "someobjfoo"

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	emptyLBStatus := v1.LoadBalancerStatus{}

	ipLBStatus := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: "127.0.0.1",
			},
		},
	}

	testCases := map[string]struct {
		status           v1.LoadBalancerStatus
		ingressClassName string
		preop            client.Object
		postop           client.Object
	}{
		"proxy: no-op add": {
			status:           emptyLBStatus,
			ingressClassName: "",
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", emptyLBStatus),
		},
		"proxy: add an IP should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", ipLBStatus),
		},
		"proxy: unset ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", emptyLBStatus),
		},
		"proxy: non-matching ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleProxyGenerator(objName, "other", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "other", emptyLBStatus),
		},
		"proxy: matching ingressclass should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleProxyGenerator(objName, "phony", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "phony", ipLBStatus),
		},
		"ingress: no-op update": {
			status:           emptyLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", emptyLBStatus),
		},
		"ingress: add an IP should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", ipLBStatus),
		},
		"ingress: unset ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", emptyLBStatus),
		},
		"ingress: not-configured ingressclass, annotation set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, ingressclass.DefaultClassName, "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, ingressclass.DefaultClassName, "", ipLBStatus),
		},
		"ingress: not-configured ingressclass, spec field set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, "", "contour", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "contour", ipLBStatus),
		},
		"ingress: not-configured ingressclass, annotation set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, "something", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "something", "", emptyLBStatus),
		},
		"ingress: not-configured ingressclass, spec field set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			preop:            simpleIngressGenerator(objName, "", "something", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "something", emptyLBStatus),
		},
		"ingress: non-matching ingressclass annotation should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "other", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "other", "", emptyLBStatus),
		},
		"ingress: non-matching ingressclass spec field should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "", "other", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "other", emptyLBStatus),
		},
		"ingress: matching ingressclass annotation should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "phony", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "phony", "", ipLBStatus),
		},
		"ingress: matching ingressclass spec field should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "", "phony", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "phony", ipLBStatus),
		},
		"ingress: non-matching ingressclass annotation should not update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "other", "phony", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "other", "phony", emptyLBStatus),
		},
		"ingress: matching ingressclass spec field should update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			preop:            simpleIngressGenerator(objName, "phony", "notcorrect", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "phony", "notcorrect", ipLBStatus),
		},
	}

	for name, tc := range testCases {
		t.Run(name+" OnAdd", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(objName, objName, tc.preop), "unable to add object to cache")

			isu := StatusAddressUpdater{
				Logger:        log,
				LBStatus:      tc.status,
				StatusUpdater: &suc,
			}
			if len(tc.ingressClassName) > 0 {
				isu.IngressClassNames = []string{tc.ingressClassName}
			}

			isu.OnAdd(tc.preop)

			newObj := suc.Get(objName, objName)
			assert.Equal(t, tc.postop, newObj)
		})

		t.Run(name+" OnUpdate", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(objName, objName, tc.preop), "unable to add object to cache")

			isu := StatusAddressUpdater{
				Logger:        log,
				LBStatus:      tc.status,
				StatusUpdater: &suc,
			}
			if len(tc.ingressClassName) > 0 {
				isu.IngressClassNames = []string{tc.ingressClassName}
			}

			isu.OnUpdate(tc.preop, tc.preop)

			newObj := suc.Get(objName, objName)
			assert.Equal(t, tc.postop, newObj)
		})
	}
}

//go:generate go run github.com/vektra/mockery/v2 --case=snake --name=Cache --srcpkg=sigs.k8s.io/controller-runtime/pkg/cache  --disable-version-string
func TestStatusAddressUpdater_Gateway(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	ipLBStatus := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: "127.0.0.1",
			},
		},
	}

	hostnameLBStatus := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: "ingress.projectcontour.io",
			},
		},
	}

	testCases := map[string]struct {
		status                     v1.LoadBalancerStatus
		gatewayClassControllerName string
		gatewayRef                 *types.NamespacedName
		preop                      *gatewayapi_v1beta1.Gateway
		postop                     *gatewayapi_v1beta1.Gateway
	}{
		"happy path (IP)": {
			status:                     ipLBStatus,
			gatewayClassControllerName: "projectcontour.io/contour",
			preop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			postop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
					Addresses: []gatewayapi_v1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayapi_v1beta1.IPAddressType),
							Value: ipLBStatus.Ingress[0].IP,
						},
					},
				},
			},
		},
		"happy path (hostname)": {
			status:                     hostnameLBStatus,
			gatewayClassControllerName: "projectcontour.io/contour",
			preop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			postop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
					Addresses: []gatewayapi_v1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayapi_v1beta1.HostnameAddressType),
							Value: hostnameLBStatus.Ingress[0].Hostname,
						},
					},
				},
			},
		},
		"Gateway not controlled by this Contour": {
			status:                     ipLBStatus,
			gatewayClassControllerName: "projectcontour.io/some-other-controller",
			preop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			postop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		},
		"Specific gateway configured, gateway does not match": {
			status:     ipLBStatus,
			gatewayRef: &types.NamespacedName{Namespace: "projectcontour", Name: "contour-gateway"},
			preop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "some-other-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			postop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "some-other-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
		},
		"Specific gateway configured, gateway matches": {
			status:     ipLBStatus,
			gatewayRef: &types.NamespacedName{Namespace: "projectcontour", Name: "contour-gateway"},
			preop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			postop: &gatewayapi_v1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "projectcontour",
					Name:      "contour-gateway",
				},
				Spec: gatewayapi_v1beta1.GatewaySpec{
					GatewayClassName: gatewayapi_v1beta1.ObjectName("contour-gatewayclass"),
				},
				Status: gatewayapi_v1beta1.GatewayStatus{
					Conditions: []metav1.Condition{
						{
							Type:   string(gatewayapi_v1beta1.GatewayConditionProgrammed),
							Status: metav1.ConditionTrue,
						},
					},
					Addresses: []gatewayapi_v1beta1.GatewayAddress{
						{
							Type:  ref.To(gatewayapi_v1beta1.IPAddressType),
							Value: ipLBStatus.Ingress[0].IP,
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name+" OnAdd", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(tc.preop.Name, tc.preop.Namespace, tc.preop), "unable to add object to cache")

			mockCache := &mocks.Cache{}
			mockCache.
				On("Get", mock.Anything, client.ObjectKey{Name: string(tc.preop.Spec.GatewayClassName)}, mock.Anything).
				Run(func(args mock.Arguments) {
					// The cache's Get function takes a pointer to a struct and updates it
					// with the data from the API server; this simulates that behavior by
					// updating the struct pointed to by the third argument with the fields
					// we care about. See Run's godoc for more info.
					args[2].(*gatewayapi_v1beta1.GatewayClass).Spec.ControllerName = gatewayapi_v1beta1.GatewayController(tc.gatewayClassControllerName)
				}).
				Return(nil)

			isu := StatusAddressUpdater{
				Logger:                log,
				GatewayControllerName: "projectcontour.io/contour",
				GatewayRef:            tc.gatewayRef,
				Cache:                 mockCache,
				LBStatus:              tc.status,
				StatusUpdater:         &suc,
			}

			isu.OnAdd(tc.preop)

			newObj := suc.Get(tc.preop.Name, tc.preop.Namespace)
			assert.Equal(t, tc.postop, newObj)
		})

		t.Run(name+" OnUpdate", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(tc.preop.Name, tc.preop.Namespace, tc.preop), "unable to add object to cache")

			mockCache := &mocks.Cache{}
			mockCache.
				On("Get", mock.Anything, client.ObjectKey{Name: string(tc.preop.Spec.GatewayClassName)}, mock.Anything).
				Run(func(args mock.Arguments) {
					// The cache's Get function takes a pointer to a struct and updates it
					// with the data from the API server; this simulates that behavior by
					// updating the struct pointed to by the third argument with the fields
					// we care about. See Run's godoc for more info.
					args[2].(*gatewayapi_v1beta1.GatewayClass).Spec.ControllerName = gatewayapi_v1beta1.GatewayController(tc.gatewayClassControllerName)
				}).
				Return(nil)

			isu := StatusAddressUpdater{
				Logger:                log,
				GatewayControllerName: "projectcontour.io/contour",
				GatewayRef:            tc.gatewayRef,
				Cache:                 mockCache,
				LBStatus:              tc.status,
				StatusUpdater:         &suc,
			}

			isu.OnUpdate(tc.preop, tc.preop)

			newObj := suc.Get(tc.preop.Name, tc.preop.Namespace)
			assert.Equal(t, tc.postop, newObj)
		})
	}
}

func simpleIngressGenerator(name, ingressClassAnnotation, ingressClassSpec string, lbstatus v1.LoadBalancerStatus) *networking_v1.Ingress {
	annotations := make(map[string]string)
	if ingressClassAnnotation != "" {
		annotations["kubernetes.io/ingress.class"] = ingressClassAnnotation
	}
	var ingressClassName *string
	if ingressClassSpec != "" {
		ingressClassName = ref.To(ingressClassSpec)
	}
	return &networking_v1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   name,
			Annotations: annotations,
		},
		Spec: networking_v1.IngressSpec{
			IngressClassName: ingressClassName,
		},
		Status: networking_v1.IngressStatus{
			LoadBalancer: coreToNetworkingLBStatus(lbstatus),
		},
	}
}

func simpleProxyGenerator(name, ingressClass string, lbstatus v1.LoadBalancerStatus) *contour_api_v1.HTTPProxy {
	return &contour_api_v1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "httpproxy",
			APIVersion: "projectcontour.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": ingressClass,
			},
		},
		Status: contour_api_v1.HTTPProxyStatus{
			LoadBalancer: lbstatus,
		},
	}
}
