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

package contour

import (
	"reflect"
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestClusterLoadAssignmentCacheRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		svc  *v1.Service
		ep   *v1.Endpoints
		want []*v2.ClusterLoadAssignment
	}{
		"simple": {
			svc: service("default", "simple", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(8080),
			}),
			ep: endpoints("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []*v2.ClusterLoadAssignment{
				clusterloadassignment("default/simple/8080", lbendpoint("192.168.183.24", 8080)),
			},
		},
		"multiple addresses": {
			svc: service("default", "httpbin-org", v1.ServicePort{
				Protocol:   "TCP",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}),
			ep: endpoints("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"23.23.247.89",
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(80),
			}),
			want: []*v2.ClusterLoadAssignment{
				clusterloadassignment("default/httpbin-org/80",
					lbendpoint("23.23.247.89", 80),
					lbendpoint("50.17.192.147", 80),
					lbendpoint("50.17.206.192", 80),
					lbendpoint("50.19.99.160", 80),
				),
			},
		},
		"named container port": {
			svc: service("default", "secure", v1.ServicePort{
				Protocol:   "TCP",
				Port:       443,
				TargetPort: intstr.FromString("https"),
			}),
			ep: endpoints("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: []v1.EndpointPort{{
					Name: "https",
					Port: 8443,
				}},
			}),
			want: []*v2.ClusterLoadAssignment{
				clusterloadassignment("default/secure/https", lbendpoint("192.168.183.24", 8443)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var cc ClusterLoadAssignmentCache
			cc.recomputeClusterLoadAssignment(tc.svc, tc.ep)
			got := cc.Values()
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}

/*
	func TestTranslatorRemoveEndpoints(t *testing.T) {
		        tests := map[string]struct {
				                setup func(*Translator)
						                ep    *v1.Endpoints
								                want  []*v2.ClusterLoadAssignment
										        }{
												                "remove existing": {
															                        setup: func(tr *Translator) {
																			                                tr.OnAdd(service("default", "simple", v1.ServicePort{
																								                                        Protocol:   "TCP",
																													                                        Port:       80,
																																		                                        TargetPort: intstr.FromInt(8080),
																																							                                }))
																																											                                tr.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
																																																                                        Addresses: addresses("192.168.183.24"),
																																																					                                        Ports:     ports(8080),
																																																										                                }))
																																																														                        },
																																																																	                        ep: endpoints("default", "simple", v1.EndpointSubset{
																																																																					                                Addresses: addresses("192.168.183.24"),
																																																																									                                Ports:     ports(8080),
																																																																													                        }),
																																																																																                        want: []*v2.ClusterLoadAssignment{},
																																																																																			                },
																																																																																					                "remove different": {
																																																																																								                        setup: func(tr *Translator) {
																																																																																												                                tr.OnAdd(service("default", "simple", v1.ServicePort{
																																																																																																	                                        Protocol:   "TCP",
																																																																																																						                                        Port:       80,
																																																																																																											                                        TargetPort: intstr.FromInt(8080),
																																																																																																																                                }))
																																																																																																																				                                tr.OnAdd(endpoints("default", "simple", v1.EndpointSubset{
																																																																																																																									                                        Addresses: addresses("192.168.183.24"),
																																																																																																																														                                        Ports:     ports(8080),
																																																																																																																																			                                }))
																																																																																																																																							                        },
																																																																																																																																										                        ep: endpoints("default", "different", v1.EndpointSubset{
																																																																																																																																														                                Addresses: addresses("192.168.183.24"),
																																																																																																																																																		                                Ports:     ports(8080),
																																																																																																																																																						                        }),
																																																																																																																																																									                        want: []*v2.ClusterLoadAssignment{
																																																																																																																																																													                                clusterloadassignment("default/simple/8080", lbendpoints(endpoint("192.168.183.24", 8080))),
																																																																																																																																																																	                        },
																																																																																																																																																																				                },
																																																																																																																																																																						                "remove non existant": {
																																																																																																																																																																									                        setup: func(*Translator) {},
																																																																																																																																																																												                        ep: endpoints("default", "simple", v1.EndpointSubset{
																																																																																																																																																																																                                Addresses: addresses("192.168.183.24"),
																																																																																																																																																																																				                                Ports:     ports(8080),
																																																																																																																																																																																								                        }),
																																																																																																																																																																																											                        want: []*v2.ClusterLoadAssignment{},
																																																																																																																																																																																														                },
																																																																																																																																																																																																        }

																																																																																																																																																																																																	        for name, tc := range tests {
																																																																																																																																																																																																			                t.Run(name, func(t *testing.T) {
																																																																																																																																																																																																						                        const NOFLAGS = 1 << 16
																																																																																																																																																																																																									                        tr := &Translator{
																																																																																																																																																																																																													                                Logger: stdlog.New(ioutil.Discard, ioutil.Discard, NOFLAGS),
																																																																																																																																																																																																																	                        }
																																																																																																																																																																																																																				                        tc.setup(tr)
																																																																																																																																																																																																																							                        tr.OnDelete(tc.ep)
																																																																																																																																																																																																																										                        got := tr.ClusterLoadAssignmentCache.Values()
																																																																																																																																																																																																																													                        if !reflect.DeepEqual(tc.want, got) {
																																																																																																																																																																																																																																	                                t.Fatalf("got: %v, want: %v", got, tc.want)
																																																																																																																																																																																																																																					                        }
																																																																																																																																																																																																																																								                })
																																																																																																																																																																																																																																										        }
																																																																																																																																																																																																																																										}
*/
