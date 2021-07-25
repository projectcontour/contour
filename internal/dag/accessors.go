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
	"github.com/projectcontour/contour/internal/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// RouteServiceName identifies a service used in a route.
type RouteServiceName struct {
	Name      string
	Namespace string
	Port      int32
}

// GetServices returns all services in the DAG.
func (dag *DAG) GetServices() map[RouteServiceName]*Service {
	getter := serviceGetter(map[RouteServiceName]*Service{})
	dag.Visit(getter.visit)
	return getter
}

// GetService returns the service in the DAG that matches the provided
// namespace, name and port, or nil if no matching service is found.
func (dag *DAG) GetService(meta types.NamespacedName, port int32) *Service {
	return dag.GetServices()[RouteServiceName{
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Port:      port,
	}]
}

// EnsureService looks for a Kubernetes service in the cache matching the provided
// namespace, name and port, and returns a DAG service for it. If a matching service
// cannot be found in the cache, an error is returned.
func (dag *DAG) EnsureService(meta types.NamespacedName, port intstr.IntOrString, cache *KubernetesCache, enableExternalNameSvc bool) (*Service, error) {
	svc, svcPort, err := cache.LookupService(meta, port)
	if err != nil {
		return nil, err
	}

	err = validateExternalName(svc, enableExternalNameSvc)
	if err != nil {
		return nil, err
	}

	if dagSvc := dag.GetService(k8s.NamespacedNameOf(svc), svcPort.Port); dagSvc != nil {
		return dagSvc, nil
	}

	dagSvc := &Service{
		Weighted: WeightedService{
			ServiceName:      svc.Name,
			ServiceNamespace: svc.Namespace,
			ServicePort:      svcPort,
			Weight:           1,
		},
		Protocol:           upstreamProtocol(svc, svcPort),
		MaxConnections:     annotation.MaxConnections(svc),
		MaxPendingRequests: annotation.MaxPendingRequests(svc),
		MaxRequests:        annotation.MaxRequests(svc),
		MaxRetries:         annotation.MaxRetries(svc),
		ExternalName:       externalName(svc),
	}
	return dagSvc, nil
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

// serviceGetter is a visitor that gets all services
// in the DAG.
type serviceGetter map[RouteServiceName]*Service

func (s serviceGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *Service:
		s[RouteServiceName{
			Name:      obj.Weighted.ServiceName,
			Namespace: obj.Weighted.ServiceNamespace,
			Port:      obj.Weighted.ServicePort.Port,
		}] = obj
	default:
		vertex.Visit(s.visit)
	}
}

// GetSecureVirtualHosts returns all secure virtual hosts in the DAG.
func (dag *DAG) GetSecureVirtualHosts() map[ListenerName]*SecureVirtualHost {
	getter := svhostGetter(map[ListenerName]*SecureVirtualHost{})
	dag.Visit(getter.visit)
	return getter
}

// GetSecureVirtualHost returns the secure virtual host in the DAG that
// matches the provided name, or nil if no matching secure virtual host
// is found.
func (dag *DAG) GetSecureVirtualHost(ln ListenerName) *SecureVirtualHost {
	return dag.GetSecureVirtualHosts()[ln]
}

// EnsureSecureVirtualHost adds a secure virtual host with the provided
// name to the DAG if it does not already exist, and returns it.
func (dag *DAG) EnsureSecureVirtualHost(ln ListenerName) *SecureVirtualHost {
	if svh := dag.GetSecureVirtualHost(ln); svh != nil {
		return svh
	}

	svh := &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name:         ln.Name,
			ListenerName: ln.ListenerName,
		},
	}
	dag.AddRoot(svh)
	return svh
}

// svhostGetter is a visitor that gets all secure virtual hosts
// in the DAG.
type svhostGetter map[ListenerName]*SecureVirtualHost

func (s svhostGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *SecureVirtualHost:
		s[ListenerName{Name: obj.Name, ListenerName: obj.VirtualHost.ListenerName}] = obj
	default:
		vertex.Visit(s.visit)
	}
}

// GetVirtualHosts returns all virtual hosts in the DAG.
func (dag *DAG) GetVirtualHosts() map[ListenerName]*VirtualHost {
	getter := vhostGetter(map[ListenerName]*VirtualHost{})
	dag.Visit(getter.visit)
	return getter
}

// GetVirtualHost returns the virtual host in the DAG that matches the
// provided name, or nil if no matching virtual host is found.
func (dag *DAG) GetVirtualHost(ln ListenerName) *VirtualHost {
	return dag.GetVirtualHosts()[ln]
}

// EnsureVirtualHost adds a virtual host with the provided name to the
// DAG if it does not already exist, and returns it.
func (dag *DAG) EnsureVirtualHost(ln ListenerName) *VirtualHost {
	if vhost := dag.GetVirtualHost(ln); vhost != nil {
		return vhost
	}

	vhost := &VirtualHost{
		Name:         ln.Name,
		ListenerName: ln.ListenerName,
	}
	dag.AddRoot(vhost)
	return vhost
}

// vhostGetter is a visitor that gets all virtual hosts
// in the DAG.
type vhostGetter map[ListenerName]*VirtualHost

func (v vhostGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *VirtualHost:
		v[ListenerName{Name: obj.Name, ListenerName: obj.ListenerName}] = obj
	default:
		vertex.Visit(v.visit)
	}
}

// GetExtensionClusters returns all extension clusters in the DAG.
func (dag *DAG) GetExtensionClusters() map[string]*ExtensionCluster {
	getter := extensionClusterGetter(map[string]*ExtensionCluster{})
	dag.Visit(getter.visit)
	return getter
}

// GetExtensionCluster returns the extension cluster in the DAG that
// matches the provided name, or nil if no matching extension cluster
//is found.
func (dag *DAG) GetExtensionCluster(name string) *ExtensionCluster {
	return dag.GetExtensionClusters()[name]
}

// extensionClusterGetter is a visitor that gets all extension clusters
// in the DAG.
type extensionClusterGetter map[string]*ExtensionCluster

func (v extensionClusterGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *ExtensionCluster:
		v[obj.Name] = obj
	default:
		vertex.Visit(v.visit)
	}
}

// validSecret returns true if the Secret contains certificate and private key material.
func validSecret(s *v1.Secret) error {
	if s.Type != v1.SecretTypeTLS {
		return fmt.Errorf("Secret type is not %q", v1.SecretTypeTLS)
	}

	if len(s.Data[v1.TLSCertKey]) == 0 {
		return fmt.Errorf("empty %q key", v1.TLSCertKey)
	}

	if len(s.Data[v1.TLSPrivateKeyKey]) == 0 {
		return fmt.Errorf("empty %q key", v1.TLSPrivateKeyKey)
	}

	return nil
}
