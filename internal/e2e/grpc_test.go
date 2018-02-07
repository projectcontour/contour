// Copyright © 2018 Heptio
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

// End to ends tests for translator to grpc operations.
package e2e

import (
	"context"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// heptio/contour#172. Updating an object from
//
// apiVersion: extensions/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   backend:
//     serviceName: kuard
//     servicePort: 80
//
// to
//
// apiVersion: extensions/v1beta1
// kind: Ingress
// metadata:
//   name: kuard
// spec:
//   rules:
//   - http:
//       paths:
//       - path: /testing
//         backend:
//           serviceName: kuard
//           servicePort: 80
//
// fails to update the virtualhost cache.
func TestEditIngress(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	meta := metav1.ObjectMeta{Name: "kuard", Namespace: "default"}

	// add default/kuard to translator.
	old := &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "kuard",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	rh.OnAdd(old)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*v2.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*v2.Route{
						route(prefixmatch("/"), cluster("default/kuard/80")),
					},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: cgrpc.RouteType,
		Nonce:   "0",
	}, fetchRDS(t, cc))

	// update old to new
	rh.OnUpdate(old, &v1beta1.Ingress{
		ObjectMeta: meta,
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/testing",
							Backend: v1beta1.IngressBackend{
								ServiceName: "kuard",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	// check that ingress_http has been updated.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*v2.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*v2.Route{
						route(prefixmatch("/testing"), cluster("default/kuard/80")),
					},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: cgrpc.RouteType,
		Nonce:   "0",
	}, fetchRDS(t, cc))
}

// heptio/contour#101
// The path /hello should point to default/hello/80 on "*"
//
// apiVersion: extensions/v1beta1
// kind: Ingress
// metadata:
//   name: hello
// spec:
//   rules:
//   - http:
// 	 paths:
//       - path: /hello
//         backend:
//           serviceName: hello
//           servicePort: 80
func TestIngressPathRouteWithoutHost(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// add default/hello to translator.
	rh.OnAdd(&v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: "/hello",
							Backend: v1beta1.IngressBackend{
								ServiceName: "hello",
								ServicePort: intstr.FromInt(80),
							},
						}},
					},
				},
			}},
		},
	})

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, &v2.RouteConfiguration{
				Name: "ingress_http",
				VirtualHosts: []*v2.VirtualHost{{
					Name:    "*",
					Domains: []string{"*"},
					Routes: []*v2.Route{
						route(prefixmatch("/hello"), cluster("default/hello/80")),
					},
				}},
			}),
			any(t, &v2.RouteConfiguration{
				Name: "ingress_https",
			}),
		},
		TypeUrl: cgrpc.RouteType,
		Nonce:   "0",
	}, fetchRDS(t, cc))
}

// heptio/contour#186
// Cluster.ServiceName and ClusterLoadAssignment.ClusterName should not be truncated.
func TestClusterLongServiceName(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	rh.OnAdd(service(
		"kuard",
		"kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r",
		v1.ServicePort{
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	))

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []*types.Any{
			any(t, &v2.Cluster{
				Name: "kuard/kbujbkuhdod66-edfcfc/8080",
				Type: v2.Cluster_EDS,
				EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
					EdsConfig:   apiconfigsource("xds_cluster"), // hard coded by initconfig
					ServiceName: "kuard/kbujbkuhdod66gjdmwmijz8xzgsx1nkfbrloezdjiulquzk4x3p0nnvpzi8r/8080",
				},
				ConnectTimeout: 250 * time.Millisecond,
				LbPolicy:       v2.Cluster_ROUND_ROBIN,
			}),
		},
		TypeUrl: cgrpc.ClusterType,
		Nonce:   "0",
	}, fetchCDS(t, cc))
}

func fetchRDS(t *testing.T, cc *grpc.ClientConn) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewRouteDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchRoutes(ctx, new(v2.DiscoveryRequest))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func fetchCDS(t *testing.T, cc *grpc.ClientConn) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewClusterDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	resp, err := rds.FetchClusters(ctx, new(v2.DiscoveryRequest))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func route(match *v2.RouteMatch, action *v2.Route_Route) *v2.Route {
	return &v2.Route{
		Match:  match,
		Action: action,
	}
}

func prefixmatch(prefix string) *v2.RouteMatch {
	return &v2.RouteMatch{
		PathSpecifier: &v2.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

func cluster(cluster string) *v2.Route_Route {
	return &v2.Route_Route{
		Route: &v2.RouteAction{
			ClusterSpecifier: &v2.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func any(t *testing.T, pb proto.Message) *types.Any {
	t.Helper()
	any, err := types.MarshalAny(pb)
	if err != nil {
		t.Fatal(err)
	}
	return any
}

func assertEqual(t *testing.T, want, got *v2.DiscoveryResponse) {
	t.Helper()
	m := proto.TextMarshaler{Compact: true, ExpandAny: true}
	a := m.Text(want)
	b := m.Text(got)
	if a != b {
		m := proto.TextMarshaler{
			Compact:   false,
			ExpandAny: true,
		}
		t.Fatalf("\nexpected:\n%v\ngot:\n%v", m.Text(want), m.Text(got))
	}
}

func service(ns, name string, ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

func apiconfigsource(clusters ...string) *v2.ConfigSource {
	return &v2.ConfigSource{
		ConfigSourceSpecifier: &v2.ConfigSource_ApiConfigSource{
			ApiConfigSource: &v2.ApiConfigSource{
				ApiType:      v2.ApiConfigSource_GRPC,
				ClusterNames: clusters,
			},
		},
	}
}
