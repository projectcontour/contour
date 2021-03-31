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
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/fixture"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
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
	ingressGVR := networking_v1.SchemeGroupVersion.WithResource("ingresses")
	proxyGVR := contour_api_v1.SchemeGroupVersion.WithResource("httpproxies")

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	emptyLBStatus := v1.LoadBalancerStatus{}
	converter, err := NewUnstructuredConverter()
	if err != nil {
		t.Error(err)
	}

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
		gvr              schema.GroupVersionResource
		preop            interface{}
		postop           interface{}
	}{
		"proxy: no-op add": {
			status:           emptyLBStatus,
			ingressClassName: "",
			gvr:              proxyGVR,
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", emptyLBStatus),
		},
		"proxy: add an IP should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              proxyGVR,
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", ipLBStatus),
		},
		"proxy: unset ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              proxyGVR,
			preop:            simpleProxyGenerator(objName, "", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "", emptyLBStatus),
		},
		"proxy: non-matching ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              proxyGVR,
			preop:            simpleProxyGenerator(objName, "other", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "other", emptyLBStatus),
		},
		"proxy: matching ingressclass should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              proxyGVR,
			preop:            simpleProxyGenerator(objName, "phony", emptyLBStatus),
			postop:           simpleProxyGenerator(objName, "phony", ipLBStatus),
		},
		"ingress: no-op update": {
			status:           emptyLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", emptyLBStatus),
		},
		"ingress: add an IP should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", ipLBStatus),
		},
		"ingress: unset ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "", emptyLBStatus),
		},
		"ingress: not-configured ingressclass, annotation set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, annotation.DEFAULT_INGRESS_CLASS_NAME, "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, annotation.DEFAULT_INGRESS_CLASS_NAME, "", ipLBStatus),
		},
		"ingress: not-configured ingressclass, spec field set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "contour", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "contour", ipLBStatus),
		},
		"ingress: not-configured ingressclass, annotation set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "something", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "something", "", emptyLBStatus),
		},
		"ingress: not-configured ingressclass, spec field set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "something", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "something", emptyLBStatus),
		},
		"ingress: non-matching ingressclass annotation should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "other", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "other", "", emptyLBStatus),
		},
		"ingress: non-matching ingressclass spec field should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "other", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "other", emptyLBStatus),
		},
		"ingress: matching ingressclass annotation should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "phony", "", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "phony", "", ipLBStatus),
		},
		"ingress: matching ingressclass spec field should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "", "phony", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "", "phony", ipLBStatus),
		},
		"ingress: non-matching ingressclass annotation should not update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "other", "phony", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "other", "phony", emptyLBStatus),
		},
		"ingress: matching ingressclass spec field should update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressGenerator(objName, "phony", "notcorrect", emptyLBStatus),
			postop:           simpleIngressGenerator(objName, "phony", "notcorrect", ipLBStatus),
		},
		"ingress v1beta1: no-op update": {
			status:           emptyLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "", emptyLBStatus),
		},
		"ingress v1beta1: add an IP should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "", ipLBStatus),
		},
		"ingress v1beta1: unset ingressclass should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "", emptyLBStatus),
		},
		"ingress v1beta1: not-configured ingressclass, annotation set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, annotation.DEFAULT_INGRESS_CLASS_NAME, "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, annotation.DEFAULT_INGRESS_CLASS_NAME, "", ipLBStatus),
		},
		"ingress v1beta1: not-configured ingressclass, spec field set to default, should update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "contour", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "contour", ipLBStatus),
		},
		"ingress v1beta1: not-configured ingressclass, annotation set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "something", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "something", "", emptyLBStatus),
		},
		"ingress v1beta1: not-configured ingressclass, spec field set, should not update": {
			status:           ipLBStatus,
			ingressClassName: "",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "something", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "something", emptyLBStatus),
		},
		"ingress v1beta1: non-matching ingressclass annotation should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "other", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "other", "", emptyLBStatus),
		},
		"ingress v1beta1: non-matching ingressclass spec field should not update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "other", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "other", emptyLBStatus),
		},
		"ingress v1beta1: matching ingressclass annotation should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "phony", "", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "phony", "", ipLBStatus),
		},
		"ingress v1beta1: matching ingressclass spec field should update": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "", "phony", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "", "phony", ipLBStatus),
		},
		"ingress v1beta1: non-matching ingressclass annotation should not update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "other", "phony", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "other", "phony", emptyLBStatus),
		},
		"ingress v1beta1: matching ingressclass spec field should update, overrides spec field": {
			status:           ipLBStatus,
			ingressClassName: "phony",
			gvr:              ingressGVR,
			preop:            simpleIngressV1Beta1Generator(objName, "phony", "notcorrect", emptyLBStatus),
			postop:           simpleIngressV1Beta1Generator(objName, "phony", "notcorrect", ipLBStatus),
		},
	}

	for name, tc := range testCases {
		t.Run(name+" OnAdd", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(objName, objName, tc.gvr, tc.preop), "unable to add object to cache")

			isu := StatusAddressUpdater{
				Logger:           log,
				LBStatus:         tc.status,
				IngressClassName: tc.ingressClassName,
				StatusUpdater:    &suc,
				Converter:        converter,
			}

			isu.OnAdd(tc.preop)

			newObj := suc.Get(objName, objName, tc.gvr)
			assert.Equal(t, tc.postop, newObj)
		})

		t.Run(name+" OnUpdate", func(t *testing.T) {
			suc := StatusUpdateCacher{}
			assert.True(t, suc.Add(objName, objName, tc.gvr, tc.preop), "unable to add object to cache")

			isu := StatusAddressUpdater{
				Logger:           log,
				LBStatus:         tc.status,
				IngressClassName: tc.ingressClassName,
				StatusUpdater:    &suc,
				Converter:        converter,
			}

			isu.OnUpdate(tc.preop, tc.preop)

			newObj := suc.Get(objName, objName, tc.gvr)
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
		ingressClassName = pointer.StringPtr(ingressClassSpec)
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
			LoadBalancer: lbstatus,
		},
	}
}

func simpleIngressV1Beta1Generator(name, ingressClassAnnotation, ingressClassSpec string, lbstatus v1.LoadBalancerStatus) *v1beta1.Ingress {
	annotations := make(map[string]string)
	if ingressClassAnnotation != "" {
		annotations["kubernetes.io/ingress.class"] = ingressClassAnnotation
	}
	var ingressClassName *string
	if ingressClassSpec != "" {
		ingressClassName = pointer.StringPtr(ingressClassSpec)
	}
	return &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ingress",
			APIVersion: "networking.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   name,
			Annotations: annotations,
		},
		Spec: v1beta1.IngressSpec{
			IngressClassName: ingressClassName,
		},
		Status: v1beta1.IngressStatus{
			LoadBalancer: lbstatus,
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
