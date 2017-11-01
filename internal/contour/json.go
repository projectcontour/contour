// Copyright © 2017 Heptio
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

// Package contour implements a REST API server for Envoy's RDS/SDS/CDS v1 JSON API
// and a gRPC API server for the xDS vs gRPC API.
package contour

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/pprof"
	"strconv"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"

	"github.com/gorilla/mux"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/log"
)

// NewJSONAPI returns a http.Handler which responds to the Envoy CDS,
// RDS, and SDS v1 REST API calls.
func NewJSONAPI(l log.Logger, ds *DataSource) http.Handler {
	r := mux.NewRouter()
	a := &jsonAPI{
		Handler:    r,
		Logger:     l,
		DataSource: ds,
	}
	r.HandleFunc("/v1/clusters/{service_cluster}/{service_node}", a.CDS)
	r.HandleFunc("/v1/registration/{namespace}/{name}/{port}", a.SDS)
	r.HandleFunc("/v1/routes/{route_config_name}/{service_cluster}/{service_node}", a.RDS)
	r.HandleFunc("/v1/listeners/{service_cluster}/{service_node}", a.LDS)

	// register pprof tracing hooks
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/{profile}", pprof.Index)
	return a
}

// DataSource provides Service, Ingress, and Endpoints caches.
type DataSource struct {
	ServiceCache
	EndpointsCache
	IngressCache
}

type jsonAPI struct {
	*DataSource
	http.Handler
	log.Logger
}

func (a *jsonAPI) CDS(w http.ResponseWriter, req *http.Request) {
	// initalise clusters to an empty slice, rather than a nil slice
	// so JSON encoding writes out {}, not nil.
	clusters := make([]envoy.Cluster, 0)
	a.ServiceCache.Each(func(s *v1.Service) {
		c, err := ServiceToClusters(s)
		if err != nil {
			a.Errorf("failed to translate service %s/%s: %v", s.ObjectMeta.Namespace, s.ObjectMeta.Name, err)
			return
		}
		clusters = append(clusters, c...)
	})
	response := struct {
		Clusters []envoy.Cluster `json:"clusters"`
	}{
		Clusters: clusters,
	}
	a.writeJSON(w, response)
}

func (a *jsonAPI) SDS(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]
	port, err := strconv.Atoi(vars["port"])
	if err != nil {
		http.Error(w, err.Error(), 504)
		return
	}

	// initalise hosts to an empty slice, rather than a nil slice
	// so JSON encoding writes out {}, not nil.
	hosts := make([]*envoy.SDSHost, 0)
	a.EndpointsCache.Each(func(e *v1.Endpoints) {
		if e.ObjectMeta.Namespace != namespace || e.ObjectMeta.Name != name {
			return
		}
		h, err := EndpointsToSDSHosts(e, port)
		if err != nil {
			a.Errorf("failed to translate endpoints %s/%s: %v", e.ObjectMeta.Namespace, e.ObjectMeta.Name, err)
			return
		}
		hosts = append(hosts, h...)
	})
	response := struct {
		Hosts []*envoy.SDSHost `json:"hosts"`
	}{
		Hosts: hosts,
	}
	a.writeJSON(w, response)
}

func (a *jsonAPI) RDS(w http.ResponseWriter, req *http.Request) {
	rc := &envoy.RouteConfig{
		VirtualHosts: make([]*envoy.VirtualHost, 0),
	}

	a.IngressCache.Each(func(i *v1beta1.Ingress) {
		v, err := IngressToVirtualHosts(i)
		if err != nil {
			a.Errorf("failed to translate ingress %s/%s: %v", i.ObjectMeta.Namespace, i.ObjectMeta.Name, err)
			return
		}
		rc.VirtualHosts = append(rc.VirtualHosts, v...)
	})

	a.writeJSON(w, rc)
}

// LDS is hard coded to return a single non TLS http manager
// bound to 0.0.0.0 on port 8080.
func (a *jsonAPI) LDS(w http.ResponseWriter, req *http.Request) {
	result := struct {
		Listeners []envoy.Listener `json:"listeners"`
	}{
		Listeners: []envoy.Listener{{
			Name:    "ingress_http",
			Address: "tcp://0.0.0.0:8080", // TODO(dfc) should come from pod.hostIP
			Filters: []envoy.Filter{
				envoy.HttpConnectionManager{
					Type: "read",
					Name: "http_connection_manager",
					Config: envoy.HttpConnectionManagerConfig{
						CodecType:        "http1",        // let's not go crazy now
						StatPrefix:       "ingress_http", // TODO(dfc) should come from pod.Name ?
						UseRemoteAddress: true,           // TODO(jbeda) Should this ever be false?
						RDS: &envoy.RDS{
							Cluster:         "rds",          // cluster name, not a url, see below
							RouteConfigName: "ingress_http", // TODO(dfc) should probably come from ingress.Name
							RefreshDelayMs:  1000,
						},
						AccessLog: []envoy.AccessLog{{
							Path: "/dev/stdout",
						}},
						Filters: []envoy.Filter{
							envoy.Router{
								Type: "decoder",
								Name: "router",
							},
						},
					},
				},
			},
		}},
	}

	a.writeJSON(w, result)
}

// writeJSON encodes v as JSON and writes it to w.
func (a *jsonAPI) writeJSON(w io.Writer, v interface{}) {
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		a.Errorf("failed to encode JSON: %v", err)
	}
}
