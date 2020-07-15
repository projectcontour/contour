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

	"github.com/google/go-cmp/cmp"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func testLogger(t *testing.T) logrus.FieldLogger {
	log := logrus.New()
	log.Out = &testWriter{t}
	return log
}

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

func TestServiceStatusLoadBalancerWatcherOnAdd(t *testing.T) {
	lbstatus := make(chan v1.LoadBalancerStatus, 1)
	sw := ServiceStatusLoadBalancerWatcher{
		ServiceName: "envoy",
		LBStatus:    lbstatus,
		Log:         testLogger(t),
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
		Log:         testLogger(t),
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
		Log:         testLogger(t),
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

type sauTestcase struct {
	status       v1.LoadBalancerStatus
	ingressClass string
	objname      string
	gvr          schema.GroupVersionResource
	preop        interface{}
	postop       interface{}
}

func TestStatusAddressUpdater_OnAdd(t *testing.T) {

	ingressGVR := v1beta1.SchemeGroupVersion.WithResource("ingresses")
	proxyGVR := projcontour.SchemeGroupVersion.WithResource("httpproxies")

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	emptyLBStatus := v1.LoadBalancerStatus{}
	converter, err := NewUnstructuredConverter()
	if err != nil {
		t.Error(err)
	}

	IPLBStatus := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: "127.0.0.1",
			},
		},
	}

	run := func(t *testing.T, name string, tc sauTestcase) {
		suc := StatusUpdateCacher{}
		if !suc.AddObject(tc.objname, tc.objname, tc.gvr, tc.preop) {
			t.Fatal("unable to add object to cache")
		}

		isu := StatusAddressUpdater{
			Logger:        log,
			LBStatus:      tc.status,
			IngressClass:  tc.ingressClass,
			StatusUpdater: &suc,
			Converter:     converter,
		}

		isu.OnAdd(tc.preop)

		newObj := suc.GetObject(tc.objname, tc.objname, tc.gvr)
		if !IsStatusEqual(tc.postop, newObj) {
			t.Fatalf("%s: Status not equal: %s\n", name, cmp.Diff(tc.postop, newObj))
		}

	}

	run(t, "ingress: no-op add", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "noop",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("noop", "", emptyLBStatus),
		postop:       simpleIngressGenerator("noop", "", emptyLBStatus),
	})

	run(t, "ingress: add an IP should update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "withIP",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("withIP", "", emptyLBStatus),
		postop:       simpleIngressGenerator("withIP", "", emptyLBStatus),
	})

	run(t, "ingress: unset ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "unsetingressclass",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("unsetingressclass", "", emptyLBStatus),
		postop:       simpleIngressGenerator("unsetingressclass", "", emptyLBStatus),
	})

	run(t, "ingress: matching ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "nonmatchingingressclass",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("nonmatchingingressclass", "other", emptyLBStatus),
		postop:       simpleIngressGenerator("nonmatchingingressclass", "other", emptyLBStatus),
	})

	run(t, "ingress: matching ingressclass should update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "matchingingressclass",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("matchingingressclass", "phony", emptyLBStatus),
		postop:       simpleIngressGenerator("matchingingressclass", "phony", IPLBStatus),
	})

	run(t, "proxy: no-op add", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "noop",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("noop", "", emptyLBStatus),
		postop:       simpleProxyGenerator("noop", "", emptyLBStatus),
	})

	run(t, "proxy: add an IP should update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "withIP",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("withIP", "", emptyLBStatus),
		postop:       simpleProxyGenerator("withIP", "", emptyLBStatus),
	})

	run(t, "proxy: unset ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "unsetingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("unsetingressclass", "", emptyLBStatus),
		postop:       simpleProxyGenerator("unsetingressclass", "", emptyLBStatus),
	})

	run(t, "proxy: matching ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "nonmatchingingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("nonmatchingingressclass", "other", emptyLBStatus),
		postop:       simpleProxyGenerator("nonmatchingingressclass", "other", emptyLBStatus),
	})

	run(t, "proxy: matching ingressclass should update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "matchingingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("matchingingressclass", "phony", emptyLBStatus),
		postop:       simpleProxyGenerator("matchingingressclass", "phony", IPLBStatus),
	})

}

func TestStatusAddressUpdater_OnUpdate(t *testing.T) {

	ingressGVR := v1beta1.SchemeGroupVersion.WithResource("ingresses")
	proxyGVR := projcontour.SchemeGroupVersion.WithResource("httpproxies")

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	emptyLBStatus := v1.LoadBalancerStatus{}
	converter, err := NewUnstructuredConverter()
	if err != nil {
		t.Error(err)
	}

	IPLBStatus := v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: "127.0.0.1",
			},
		},
	}

	run := func(t *testing.T, name string, tc sauTestcase) {
		suc := StatusUpdateCacher{}
		if !suc.AddObject(tc.objname, tc.objname, tc.gvr, tc.preop) {
			t.Fatal("unable to add object to cache")
		}

		isu := StatusAddressUpdater{
			Logger:        log,
			LBStatus:      tc.status,
			IngressClass:  tc.ingressClass,
			StatusUpdater: &suc,
			Converter:     converter,
		}

		isu.OnUpdate(tc.preop, tc.preop)

		newObj := suc.GetObject(tc.objname, tc.objname, tc.gvr)
		if !IsStatusEqual(tc.postop, newObj) {
			t.Fatalf("%s: Status not equal: %s\n", name, cmp.Diff(tc.postop, newObj))
		}

	}

	run(t, "ingress: no-op update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "noop",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("noop", "", emptyLBStatus),
		postop:       simpleIngressGenerator("noop", "", emptyLBStatus),
	})

	run(t, "ingress: add an IP should update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "withIP",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("withIP", "", emptyLBStatus),
		postop:       simpleIngressGenerator("withIP", "", emptyLBStatus),
	})

	run(t, "ingress: unset ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "unsetingressclass",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("unsetingressclass", "", emptyLBStatus),
		postop:       simpleIngressGenerator("unsetingressclass", "", emptyLBStatus),
	})

	run(t, "ingress: matching ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		gvr:          ingressGVR,
		objname:      "nonmatchingingressclass",
		preop:        simpleIngressGenerator("nonmatchingingressclass", "other", emptyLBStatus),
		postop:       simpleIngressGenerator("nonmatchingingressclass", "other", emptyLBStatus),
	})

	run(t, "ingress: matching ingressclass should update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "matchingingressclass",
		gvr:          ingressGVR,
		preop:        simpleIngressGenerator("matchingingressclass", "phony", emptyLBStatus),
		postop:       simpleIngressGenerator("matchingingressclass", "phony", IPLBStatus),
	})

	run(t, "proxy: no-op update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "noop",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("noop", "", emptyLBStatus),
		postop:       simpleProxyGenerator("noop", "", emptyLBStatus),
	})

	run(t, "proxy: add an IP should update", sauTestcase{
		status:       emptyLBStatus,
		ingressClass: "",
		objname:      "withIP",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("withIP", "", emptyLBStatus),
		postop:       simpleProxyGenerator("withIP", "", emptyLBStatus),
	})

	run(t, "proxy: unset ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "unsetingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("unsetingressclass", "", emptyLBStatus),
		postop:       simpleProxyGenerator("unsetingressclass", "", emptyLBStatus),
	})

	run(t, "proxy: matching ingressclass should not update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "nonmatchingingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("nonmatchingingressclass", "other", emptyLBStatus),
		postop:       simpleProxyGenerator("nonmatchingingressclass", "other", emptyLBStatus),
	})

	run(t, "proxy: matching ingressclass should update", sauTestcase{
		status:       IPLBStatus,
		ingressClass: "phony",
		objname:      "matchingingressclass",
		gvr:          proxyGVR,
		preop:        simpleProxyGenerator("matchingingressclass", "phony", emptyLBStatus),
		postop:       simpleProxyGenerator("matchingingressclass", "phony", IPLBStatus),
	})
}

func simpleIngressGenerator(name, ingressClass string, lbstatus v1.LoadBalancerStatus) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": ingressClass,
			},
		},

		Status: v1beta1.IngressStatus{
			LoadBalancer: lbstatus,
		},
	}
}

func simpleProxyGenerator(name, ingressClass string, lbstatus v1.LoadBalancerStatus) *projcontour.HTTPProxy {
	return &projcontour.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: name,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": ingressClass,
			},
		},

		Status: projcontour.HTTPProxyStatus{
			LoadBalancer: lbstatus,
		},
	}
}
