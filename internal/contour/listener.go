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

package contour

import (
	"path/filepath"

	v2 "github.com/envoyproxy/go-control-plane/api"
	"github.com/gogo/protobuf/types"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
)

const (
	ENVOY_HTTP_LISTENER        = "ingress_http"
	ENVOY_HTTPS_LISTENER       = "ingress_https"
	DEFAULT_HTTP_LISTENER_PORT = 8080

	router     = "envoy.router"
	httpFilter = "envoy.http_connection_manager"
	accessLog  = "envoy.file_access_log"
)

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	// Envoy's HTTP (non TLS) listener port.
	// If not set, defaults to DEFAULT_HTTP_LISTENER_PORT.
	HTTPListenerPort int

	listenerCache
	Cond
}

// recomputeListeners recomputes the ingress_http and ingress_https listeners
// and notifies the watchers any change.
func (lc *ListenerCache) recomputeListeners(ingresses map[metadata]*v1beta1.Ingress, secrets map[metadata]*v1.Secret) {
	add, remove := lc.recomputeListener0(ingresses)                   // recompute ingress_http
	ssladd, sslremove := lc.recomputeTLSListener0(ingresses, secrets) // recompute ingress_https

	add = append(add, ssladd...)
	remove = append(remove, sslremove...)
	lc.Add(add...)
	lc.Remove(remove...)

	if len(add) > 0 || len(remove) > 0 {
		lc.Notify()
	}
}

// recomputeTLSListener recomputes the ingress_https listener and notifies the watchers
// of any change.
func (lc *ListenerCache) recomputeTLSListener(ingresses map[metadata]*v1beta1.Ingress, secrets map[metadata]*v1.Secret) {
	ssladd, sslremove := lc.recomputeTLSListener0(ingresses, secrets) // recompute ingress_https
	lc.Add(ssladd...)
	lc.Remove(sslremove...)
	if len(ssladd) > 0 || len(sslremove) > 0 {
		lc.Notify()
	}
}

// recomputeListener recomputes the non SSL listener for port 8080 using the list of ingresses provided.
// recomputeListener returns a slice of listeners to be added to the cache, and a slice of names of listeners
// to be removed.
func (lc *ListenerCache) recomputeListener0(ingresses map[metadata]*v1beta1.Ingress) ([]*v2.Listener, []string) {
	l := listener(ENVOY_HTTP_LISTENER, "0.0.0.0", lc.httpListenerPort())

	var valid int
	for _, i := range ingresses {
		if validIngress(i) {
			valid++
		}
	}
	if valid > 0 {
		l.FilterChains = []*v2.FilterChain{{
			Filters: []*v2.Filter{
				httpfilter(ENVOY_HTTP_LISTENER),
			},
		}}
	}
	// TODO(dfc) some annotations may require the Ingress to no appear on
	// port 80, therefore may result in an empty effective set of ingresses.
	switch len(l.FilterChains) {
	case 0:
		// no ingresses registered, remove this listener.
		return nil, []string{l.Name}
	default:
		// at least one ingress registered, refresh listener
		return []*v2.Listener{l}, nil
	}
}

// httpListenerPort returns the port for the HTTP (non TLS)
// listener or DEFAULT_HTTP_LISTENER_PORT if not configured.
func (lc *ListenerCache) httpListenerPort() uint32 {
	if lc.HTTPListenerPort != 0 {
		return uint32(lc.HTTPListenerPort)
	}
	return DEFAULT_HTTP_LISTENER_PORT
}

// validIngress returns true if this is a valid non ssl ingress object.
// ingresses are invalid if they contain annotations which exclude them from
// the ingress_http listener.
func validIngress(i *v1beta1.Ingress) bool {
	if i.Annotations["kubernetes.io/ingress.allow-http"] == "false" {
		return false
	}
	return true
}

// recomputeTLSListener0 recomputes the SSL listener for port 8443
// using the list of ingresses and secrets provided.
// recomputeListener returns a slice of listeners to be added to the cache,
// and a slice of names of listeners to be removed. If the list of
// TLS enabled listeners is zero, the listener is removed.
func (lc *ListenerCache) recomputeTLSListener0(ingresses map[metadata]*v1beta1.Ingress, secrets map[metadata]*v1.Secret) ([]*v2.Listener, []string) {
	l := listener(ENVOY_HTTPS_LISTENER, "0.0.0.0", 8443)
	filters := []*v2.Filter{
		httpfilter(ENVOY_HTTPS_LISTENER),
	}
	for _, i := range ingresses {
		if !validTLSIngress(i) {
			continue
		}
		for _, tls := range i.Spec.TLS {
			_, ok := secrets[metadata{name: tls.SecretName, namespace: i.Namespace}]
			if !ok {
				// no secret for this ingress yet, skip it
				continue
			}
			fc := &v2.FilterChain{
				FilterChainMatch: &v2.FilterChainMatch{
					SniDomains: tls.Hosts,
				},
				TlsContext: tlscontext(i.Namespace, tls.SecretName),
				Filters:    filters,
			}
			l.FilterChains = append(l.FilterChains, fc)
		}
	}

	switch len(l.FilterChains) {
	case 0:
		// no tls ingresses registered, remove the listener
		return nil, []string{l.Name}
	default:
		// at least one tls ingress registered, refresh listener
		return []*v2.Listener{l}, nil
	}
}

// validTLSIngress returns true if this is a valid ssl ingress object.
// ingresses are invalid if they contain annotations, or are missing information
// which excludes them from the ingress_https listener.
func validTLSIngress(i *v1beta1.Ingress) bool {
	if len(i.Spec.TLS) == 0 {
		// this ingress does not use TLS, skip it
		return false
	}
	return true
}

func listener(name, address string, port uint32) *v2.Listener {
	return &v2.Listener{
		Name:    name, // TODO(dfc) should come from the name of the service port
		Address: socketaddress(address, port),
	}
}

func socketaddress(address string, port uint32) *v2.Address {
	return &v2.Address{
		Address: &v2.Address_SocketAddress{
			SocketAddress: &v2.SocketAddress{
				Protocol: v2.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &v2.SocketAddress_PortValue{
					PortValue: port,
				},
			},
		},
	}
}

func tlscontext(namespace, name string) *v2.DownstreamTlsContext {
	const base = "/config/ssl"
	return &v2.DownstreamTlsContext{
		CommonTlsContext: &v2.CommonTlsContext{
			TlsCertificates: []*v2.TlsCertificate{{
				CertificateChain: &v2.DataSource{
					&v2.DataSource_Filename{
						Filename: filepath.Join(base, namespace, name, v1.TLSCertKey),
					},
				},
				PrivateKey: &v2.DataSource{
					&v2.DataSource_Filename{
						Filename: filepath.Join(base, namespace, name, v1.TLSPrivateKeyKey),
					},
				},
			}},
		},
	}
}

func httpfilter(routename string) *v2.Filter {
	return &v2.Filter{
		Name: httpFilter,
		Config: &types.Struct{
			Fields: map[string]*types.Value{
				"codec_type":  sv("auto"),
				"stat_prefix": sv(routename),
				"rds": st(map[string]*types.Value{
					"route_config_name": sv(routename),
					"config_source": st(map[string]*types.Value{
						"api_config_source": st(map[string]*types.Value{
							"api_type": sv("grpc"),
							"cluster_name": lv(
								sv("xds_cluster"),
							),
						}),
					}),
				}),
				"http_filters": lv(
					st(map[string]*types.Value{
						"name": sv(router),
					}),
				),
				"access_log": st(map[string]*types.Value{
					"name": sv(accessLog),
					"config": st(map[string]*types.Value{
						"path": sv("/dev/stdout"),
					}),
				}),
				"use_remote_address": bv(true), // TODO(jbeda) should this ever be false?
			},
		},
	}
}

func sv(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func bv(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func st(m map[string]*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: m}}}
}
func lv(v ...*types.Value) *types.Value {
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: v}}}
}
