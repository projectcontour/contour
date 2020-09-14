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
func GetServices(dag *DAG) map[RouteServiceName]*Service {
	getter := serviceGetter(map[RouteServiceName]*Service{})
	dag.Visit(getter.visit)
	return getter
}

// GetService returns the service in the DAG that matches the provided
// namespace, name and port, or nil if no matching service is found.
func GetService(meta types.NamespacedName, port int32, dag *DAG) *Service {
	return GetServices(dag)[RouteServiceName{
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Port:      port,
	}]
}

// EnsureService looks for a Kubernetes service in the cache matching the provided
// namespace, name and port, adds it to the DAG if it does not already exist, and
/// returns it. If a matching service cannot be found in the cache, an error is
// returned.
func EnsureService(meta types.NamespacedName, port intstr.IntOrString, dag *DAG, cache *KubernetesCache) (*Service, error) {
	svc, svcPort, err := cache.LookupService(meta, port)
	if err != nil {
		return nil, err
	}

	if dagSvc := GetService(k8s.NamespacedNameOf(svc), svcPort.Port, dag); dagSvc != nil {
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
	dag.AddRoot(dagSvc)
	return dagSvc, nil
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
func GetSecureVirtualHosts(dag *DAG) map[string]*SecureVirtualHost {
	getter := svhostGetter(map[string]*SecureVirtualHost{})
	dag.Visit(getter.visit)
	return getter
}

// GetSecureVirtualHost returns the secure virtual host in the DAG that
// matches the provided name, or nil if no matching secure virtual host
// is found.
func GetSecureVirtualHost(name string, dag *DAG) *SecureVirtualHost {
	return GetSecureVirtualHosts(dag)[name]
}

// EnsureSecureVirtualHost adds a secure virtual host with the provided
// name to the DAG if it does not already exist, and returns it.
func EnsureSecureVirtualHost(name string, dag *DAG) *SecureVirtualHost {
	if svh := GetSecureVirtualHost(name, dag); svh != nil {
		return svh
	}

	svh := &SecureVirtualHost{
		VirtualHost: VirtualHost{
			Name: name,
		},
	}
	dag.AddRoot(svh)
	return svh
}

// svhostGetter is a visitor that gets all secure virtual hosts
// in the DAG.
type svhostGetter map[string]*SecureVirtualHost

func (s svhostGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *SecureVirtualHost:
		s[obj.Name] = obj
	default:
		vertex.Visit(s.visit)
	}
}

// GetVirtualHosts returns all virtual hosts in the DAG.
func GetVirtualHosts(dag *DAG) map[string]*VirtualHost {
	getter := vhostGetter(map[string]*VirtualHost{})
	dag.Visit(getter.visit)
	return getter
}

// GetVirtualHost returns the virtual host in the DAG that matches the
// provided name, or nil if no matching virtual host is found.
func GetVirtualHost(name string, dag *DAG) *VirtualHost {
	return GetVirtualHosts(dag)[name]
}

// EnsureVirtualHost adds a virtual host with the provided name to the
// DAG if it does not already exist, and returns it.
func EnsureVirtualHost(name string, dag *DAG) *VirtualHost {
	if vhost := GetVirtualHost(name, dag); vhost != nil {
		return vhost
	}

	vhost := &VirtualHost{
		Name: name,
	}
	dag.AddRoot(vhost)
	return vhost
}

// vhostGetter is a visitor that gets all virtual hosts
// in the DAG.
type vhostGetter map[string]*VirtualHost

func (v vhostGetter) visit(vertex Vertex) {
	switch obj := vertex.(type) {
	case *VirtualHost:
		v[obj.Name] = obj
	default:
		vertex.Visit(v.visit)
	}
}

// GetExtensionClusters returns all extension clusters in the DAG.
func GetExtensionClusters(dag *DAG) map[string]*ExtensionCluster {
	getter := extensionClusterGetter(map[string]*ExtensionCluster{})
	dag.Visit(getter.visit)
	return getter
}

// GetExtensionCluster returns the extension cluster in the DAG that
// matches the provided name, or nil if no matching extension cluster
//is found.
func GetExtensionCluster(name string, dag *DAG) *ExtensionCluster {
	return GetExtensionClusters(dag)[name]
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
