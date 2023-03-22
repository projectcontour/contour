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

package dag

import (
	"fmt"
	"strconv"

	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/xds"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EnsureService looks for a Kubernetes service in the cache matching the provided
// namespace, name and port, and returns a DAG service for it. If a matching service
// cannot be found in the cache, an error is returned.
func (d *DAG) EnsureService(meta types.NamespacedName, port int, healthPort int, cache *KubernetesCache, enableExternalNameSvc bool) (*Service, error) {
	svc, svcPort, err := cache.LookupService(meta, intstr.FromInt(port))
	if err != nil {
		return nil, err
	}

	healthSvcPort := svcPort
	if healthPort != 0 && healthPort != port {
		_, healthSvcPort, err = cache.LookupService(meta, intstr.FromInt(healthPort))
		if err != nil {
			return nil, err
		}
	}

	err = validateExternalName(svc, enableExternalNameSvc)
	if err != nil {
		return nil, err
	}

	// There's no need to walk the DAG to look for a matching
	// existing Service here. They're terminal nodes in the DAG
	// so nothing is getting attached to them, and when used
	// to generate an Envoy cluster any copy of this info will
	// do. Doing a DAG walk to look for them is also very
	// inefficient and can cause performance isuses at scale
	// (see https://github.com/projectcontour/contour/issues/4058).
	return &Service{
		Weighted: WeightedService{
			ServiceName:      svc.Name,
			ServiceNamespace: svc.Namespace,
			ServicePort:      svcPort,
			HealthPort:       healthSvcPort,
			Weight:           1,
		},
		Protocol:           upstreamProtocol(svc, svcPort),
		MaxConnections:     annotation.MaxConnections(svc),
		MaxPendingRequests: annotation.MaxPendingRequests(svc),
		MaxRequests:        annotation.MaxRequests(svc),
		MaxRetries:         annotation.MaxRetries(svc),
		ExternalName:       externalName(svc),
	}, nil
}

func validateExternalName(svc *v1.Service, enableExternalNameSvc bool) error {

	// If this isn't an ExternalName Service, we're all good here.
	en := externalName(svc)
	if en == "" {
		return nil
	}

	// If ExternalNames are disabled, then we don't want to add this to the DAG.
	if !enableExternalNameSvc {
		return fmt.Errorf("%s/%s is an ExternalName service, these are not currently enabled. See the config.enableExternalNameService config file setting", svc.Namespace, svc.Name)
	}

	// Check against a list of known localhost names, using a map to approximate a set.
	// TODO(youngnick) This is a very porous hack, and we should probably look into doing a DNS
	// lookup to check what the externalName resolves to, but I'm worried about the
	// performance impact of doing one or more DNS lookups per DAG run, so we're
	// going to go with a specific blocklist for now.
	localhostNames := map[string]struct{}{
		"localhost":               {},
		"localhost.localdomain":   {},
		"local.projectcontour.io": {},
	}

	_, localhost := localhostNames[en]
	if localhost {
		return fmt.Errorf("%s/%s is an ExternalName service that points to localhost, this is not allowed", svc.Namespace, svc.Name)
	}

	return nil
}

func upstreamProtocol(svc *v1.Service, port v1.ServicePort) string {
	up := annotation.ParseUpstreamProtocols(svc.Annotations)
	protocol := up[port.Name]
	if protocol == "" {
		protocol = up[strconv.Itoa(int(port.Port))]
	}
	return protocol
}

func externalName(svc *v1.Service) string {
	if svc.Spec.Type != v1.ServiceTypeExternalName {
		return ""
	}
	return svc.Spec.ExternalName
}

// GetSecureVirtualHost returns the secure virtual host in the DAG that
// matches the provided name, or nil if no matching secure virtual host
// is found.
func (d *DAG) GetSecureVirtualHost(listener, hostname string) *SecureVirtualHost {
	return d.Listeners[listener].svhostsByName[hostname]
}

// EnsureSecureVirtualHost adds a secure virtual host with the provided
// name to the DAG if it does not already exist, and returns it.
func (d *DAG) EnsureSecureVirtualHost(listener, hostname string) *SecureVirtualHost {
	if svh := d.GetSecureVirtualHost(listener, hostname); svh != nil {
		return svh
	}

	svh := &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name: hostname,
		},
	}

	d.Listeners[listener].SecureVirtualHosts = append(d.Listeners[listener].SecureVirtualHosts, svh)
	d.Listeners[listener].svhostsByName[svh.Name] = svh
	return svh
}

// GetVirtualHost returns the virtual host in the DAG that matches the
// provided name, or nil if no matching virtual host is found.
func (d *DAG) GetVirtualHost(listener, hostname string) *VirtualHost {
	return d.Listeners[listener].vhostsByName[hostname]
}

// EnsureVirtualHost adds a virtual host with the provided name to the
// DAG if it does not already exist, and returns it.
func (d *DAG) EnsureVirtualHost(listener, hostname string) *VirtualHost {
	if vhost := d.GetVirtualHost(listener, hostname); vhost != nil {
		return vhost
	}

	vhost := &VirtualHost{
		Name: hostname,
	}

	d.Listeners[listener].VirtualHosts = append(d.Listeners[listener].VirtualHosts, vhost)
	d.Listeners[listener].vhostsByName[vhost.Name] = vhost
	return vhost
}

func (d *DAG) GetClusters() []*Cluster {
	var res []*Cluster

	for _, listener := range d.Listeners {
		for _, vhost := range listener.VirtualHosts {
			for _, route := range vhost.Routes {
				res = append(res, route.Clusters...)

				if route.MirrorPolicy != nil && route.MirrorPolicy.Cluster != nil {
					res = append(res, route.MirrorPolicy.Cluster)
				}
			}
		}

		for _, vhost := range listener.SecureVirtualHosts {
			for _, route := range vhost.Routes {
				res = append(res, route.Clusters...)

				if route.MirrorPolicy != nil && route.MirrorPolicy.Cluster != nil {
					res = append(res, route.MirrorPolicy.Cluster)
				}
			}

			if vhost.TCPProxy != nil {
				res = append(res, vhost.TCPProxy.Clusters...)
			}
		}
	}

	return res
}

func (d *DAG) GetDNSNameClusters() []*DNSNameCluster {
	var res []*DNSNameCluster

	for _, listener := range d.Listeners {
		for _, svhost := range listener.SecureVirtualHosts {
			for _, provider := range svhost.JWTProviders {
				provider := provider
				res = append(res, &provider.RemoteJWKS.Cluster)
			}
		}
	}

	return res
}

func (d *DAG) GetServiceClusters() []*ServiceCluster {
	var res []*ServiceCluster

	for _, cluster := range d.GetClusters() {
		// A Service has only one WeightedService entry. Fake up a
		// ServiceCluster so that the visitor can pretend to not
		// know this.
		c := &ServiceCluster{
			ClusterName: xds.ClusterLoadAssignmentName(
				types.NamespacedName{
					Name:      cluster.Upstream.Weighted.ServiceName,
					Namespace: cluster.Upstream.Weighted.ServiceNamespace,
				},
				cluster.Upstream.Weighted.ServicePort.Name),
			Services: []WeightedService{
				cluster.Upstream.Weighted,
			},
		}

		res = append(res, c)
	}

	for _, ec := range d.ExtensionClusters {
		res = append(res, &ec.Upstream)
	}

	return res
}

// GetExtensionClusters returns all extension clusters in the DAG.
func (d *DAG) GetExtensionClusters() map[string]*ExtensionCluster {
	// TODO for DAG consumers, this should iterate
	// over Listeners.SecureVirtualHosts and get
	// AuthorizationServices.
	res := map[string]*ExtensionCluster{}
	for _, ec := range d.ExtensionClusters {
		res[ec.Name] = ec
	}
	return res
}

func (d *DAG) GetSecrets() []*Secret {
	var res []*Secret
	for _, l := range d.Listeners {
		for _, svh := range l.SecureVirtualHosts {
			if svh.Secret != nil {
				res = append(res, svh.Secret)
			}
			if svh.FallbackCertificate != nil {
				res = append(res, svh.FallbackCertificate)
			}
		}
	}

	for _, c := range d.GetClusters() {
		if c.ClientCertificate != nil {
			res = append(res, c.ClientCertificate)
		}
	}

	return res
}

// GetExtensionCluster returns the extension cluster in the DAG that
// matches the provided name, or nil if no matching extension cluster
// is found.
func (d *DAG) GetExtensionCluster(name string) *ExtensionCluster {
	for _, ec := range d.ExtensionClusters {
		if ec.Name == name {
			return ec
		}
	}

	return nil
}

func (d *DAG) GetVirtualHostRoutes() map[*VirtualHost][]*Route {
	res := map[*VirtualHost][]*Route{}

	for _, l := range d.Listeners {
		for _, vhost := range l.VirtualHosts {
			var routes []*Route
			for _, r := range vhost.Routes {
				routes = append(routes, r)
			}
			if len(routes) > 0 {
				res[vhost] = routes
			}
		}
	}

	return res
}

func (d *DAG) GetSecureVirtualHostRoutes() map[*SecureVirtualHost][]*Route {
	res := map[*SecureVirtualHost][]*Route{}

	for _, l := range d.Listeners {
		for _, vhost := range l.SecureVirtualHosts {
			var routes []*Route
			for _, r := range vhost.Routes {
				routes = append(routes, r)
			}
			if len(routes) > 0 {
				res[vhost] = routes
			}
		}
	}

	return res
}
