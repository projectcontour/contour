//go:build !ignore_autogenerated

/*
Copyright Project Contour Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"github.com/projectcontour/contour/apis/projectcontour/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in AccessLogJSONFields) DeepCopyInto(out *AccessLogJSONFields) {
	{
		in := &in
		*out = make(AccessLogJSONFields, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccessLogJSONFields.
func (in AccessLogJSONFields) DeepCopy() AccessLogJSONFields {
	if in == nil {
		return nil
	}
	out := new(AccessLogJSONFields)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CircuitBreakers) DeepCopyInto(out *CircuitBreakers) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CircuitBreakers.
func (in *CircuitBreakers) DeepCopy() *CircuitBreakers {
	if in == nil {
		return nil
	}
	out := new(CircuitBreakers)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterParameters) DeepCopyInto(out *ClusterParameters) {
	*out = *in
	if in.MaxRequestsPerConnection != nil {
		in, out := &in.MaxRequestsPerConnection, &out.MaxRequestsPerConnection
		*out = new(uint32)
		**out = **in
	}
	if in.PerConnectionBufferLimitBytes != nil {
		in, out := &in.PerConnectionBufferLimitBytes, &out.PerConnectionBufferLimitBytes
		*out = new(uint32)
		**out = **in
	}
	if in.GlobalCircuitBreakerDefaults != nil {
		in, out := &in.GlobalCircuitBreakerDefaults, &out.GlobalCircuitBreakerDefaults
		*out = new(CircuitBreakers)
		**out = **in
	}
	if in.UpstreamTLS != nil {
		in, out := &in.UpstreamTLS, &out.UpstreamTLS
		*out = new(EnvoyTLS)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterParameters.
func (in *ClusterParameters) DeepCopy() *ClusterParameters {
	if in == nil {
		return nil
	}
	out := new(ClusterParameters)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourConfiguration) DeepCopyInto(out *ContourConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourConfiguration.
func (in *ContourConfiguration) DeepCopy() *ContourConfiguration {
	if in == nil {
		return nil
	}
	out := new(ContourConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ContourConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourConfigurationList) DeepCopyInto(out *ContourConfigurationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ContourConfiguration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourConfigurationList.
func (in *ContourConfigurationList) DeepCopy() *ContourConfigurationList {
	if in == nil {
		return nil
	}
	out := new(ContourConfigurationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ContourConfigurationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourConfigurationSpec) DeepCopyInto(out *ContourConfigurationSpec) {
	*out = *in
	if in.XDSServer != nil {
		in, out := &in.XDSServer, &out.XDSServer
		*out = new(XDSServerConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Ingress != nil {
		in, out := &in.Ingress, &out.Ingress
		*out = new(IngressConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Debug != nil {
		in, out := &in.Debug, &out.Debug
		*out = new(DebugConfig)
		**out = **in
	}
	if in.Health != nil {
		in, out := &in.Health, &out.Health
		*out = new(HealthConfig)
		**out = **in
	}
	if in.Envoy != nil {
		in, out := &in.Envoy, &out.Envoy
		*out = new(EnvoyConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Gateway != nil {
		in, out := &in.Gateway, &out.Gateway
		*out = new(GatewayConfig)
		**out = **in
	}
	if in.HTTPProxy != nil {
		in, out := &in.HTTPProxy, &out.HTTPProxy
		*out = new(HTTPProxyConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.EnableExternalNameService != nil {
		in, out := &in.EnableExternalNameService, &out.EnableExternalNameService
		*out = new(bool)
		**out = **in
	}
	if in.GlobalExternalAuthorization != nil {
		in, out := &in.GlobalExternalAuthorization, &out.GlobalExternalAuthorization
		*out = new(v1.AuthorizationServer)
		(*in).DeepCopyInto(*out)
	}
	if in.RateLimitService != nil {
		in, out := &in.RateLimitService, &out.RateLimitService
		*out = new(RateLimitServiceConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Policy != nil {
		in, out := &in.Policy, &out.Policy
		*out = new(PolicyConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = new(MetricsConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Tracing != nil {
		in, out := &in.Tracing, &out.Tracing
		*out = new(TracingConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.FeatureFlags != nil {
		in, out := &in.FeatureFlags, &out.FeatureFlags
		*out = make(FeatureFlags, len(*in))
		copy(*out, *in)
	}
	if in.GlobalExternalProcessing != nil {
		in, out := &in.GlobalExternalProcessing, &out.GlobalExternalProcessing
		*out = new(v1.ExternalProcessing)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourConfigurationSpec.
func (in *ContourConfigurationSpec) DeepCopy() *ContourConfigurationSpec {
	if in == nil {
		return nil
	}
	out := new(ContourConfigurationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourConfigurationStatus) DeepCopyInto(out *ContourConfigurationStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.DetailedCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourConfigurationStatus.
func (in *ContourConfigurationStatus) DeepCopy() *ContourConfigurationStatus {
	if in == nil {
		return nil
	}
	out := new(ContourConfigurationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourDeployment) DeepCopyInto(out *ContourDeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourDeployment.
func (in *ContourDeployment) DeepCopy() *ContourDeployment {
	if in == nil {
		return nil
	}
	out := new(ContourDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ContourDeployment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourDeploymentList) DeepCopyInto(out *ContourDeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ContourDeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourDeploymentList.
func (in *ContourDeploymentList) DeepCopy() *ContourDeploymentList {
	if in == nil {
		return nil
	}
	out := new(ContourDeploymentList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ContourDeploymentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourDeploymentSpec) DeepCopyInto(out *ContourDeploymentSpec) {
	*out = *in
	if in.Contour != nil {
		in, out := &in.Contour, &out.Contour
		*out = new(ContourSettings)
		(*in).DeepCopyInto(*out)
	}
	if in.Envoy != nil {
		in, out := &in.Envoy, &out.Envoy
		*out = new(EnvoySettings)
		(*in).DeepCopyInto(*out)
	}
	if in.RuntimeSettings != nil {
		in, out := &in.RuntimeSettings, &out.RuntimeSettings
		*out = new(ContourConfigurationSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.ResourceLabels != nil {
		in, out := &in.ResourceLabels, &out.ResourceLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourDeploymentSpec.
func (in *ContourDeploymentSpec) DeepCopy() *ContourDeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(ContourDeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourDeploymentStatus) DeepCopyInto(out *ContourDeploymentStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourDeploymentStatus.
func (in *ContourDeploymentStatus) DeepCopy() *ContourDeploymentStatus {
	if in == nil {
		return nil
	}
	out := new(ContourDeploymentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ContourSettings) DeepCopyInto(out *ContourSettings) {
	*out = *in
	if in.NodePlacement != nil {
		in, out := &in.NodePlacement, &out.NodePlacement
		*out = new(NodePlacement)
		(*in).DeepCopyInto(*out)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.Deployment != nil {
		in, out := &in.Deployment, &out.Deployment
		*out = new(DeploymentSettings)
		(*in).DeepCopyInto(*out)
	}
	if in.PodAnnotations != nil {
		in, out := &in.PodAnnotations, &out.PodAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.WatchNamespaces != nil {
		in, out := &in.WatchNamespaces, &out.WatchNamespaces
		*out = make([]v1.Namespace, len(*in))
		copy(*out, *in)
	}
	if in.DisabledFeatures != nil {
		in, out := &in.DisabledFeatures, &out.DisabledFeatures
		*out = make([]v1.Feature, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ContourSettings.
func (in *ContourSettings) DeepCopy() *ContourSettings {
	if in == nil {
		return nil
	}
	out := new(ContourSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomTag) DeepCopyInto(out *CustomTag) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomTag.
func (in *CustomTag) DeepCopy() *CustomTag {
	if in == nil {
		return nil
	}
	out := new(CustomTag)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DaemonSetSettings) DeepCopyInto(out *DaemonSetSettings) {
	*out = *in
	if in.UpdateStrategy != nil {
		in, out := &in.UpdateStrategy, &out.UpdateStrategy
		*out = new(appsv1.DaemonSetUpdateStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DaemonSetSettings.
func (in *DaemonSetSettings) DeepCopy() *DaemonSetSettings {
	if in == nil {
		return nil
	}
	out := new(DaemonSetSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DebugConfig) DeepCopyInto(out *DebugConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DebugConfig.
func (in *DebugConfig) DeepCopy() *DebugConfig {
	if in == nil {
		return nil
	}
	out := new(DebugConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeploymentSettings) DeepCopyInto(out *DeploymentSettings) {
	*out = *in
	if in.Strategy != nil {
		in, out := &in.Strategy, &out.Strategy
		*out = new(appsv1.DeploymentStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeploymentSettings.
func (in *DeploymentSettings) DeepCopy() *DeploymentSettings {
	if in == nil {
		return nil
	}
	out := new(DeploymentSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoyConfig) DeepCopyInto(out *EnvoyConfig) {
	*out = *in
	if in.Listener != nil {
		in, out := &in.Listener, &out.Listener
		*out = new(EnvoyListenerConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Service != nil {
		in, out := &in.Service, &out.Service
		*out = new(NamespacedName)
		**out = **in
	}
	if in.HTTPListener != nil {
		in, out := &in.HTTPListener, &out.HTTPListener
		*out = new(EnvoyListener)
		**out = **in
	}
	if in.HTTPSListener != nil {
		in, out := &in.HTTPSListener, &out.HTTPSListener
		*out = new(EnvoyListener)
		**out = **in
	}
	if in.Health != nil {
		in, out := &in.Health, &out.Health
		*out = new(HealthConfig)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = new(MetricsConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.ClientCertificate != nil {
		in, out := &in.ClientCertificate, &out.ClientCertificate
		*out = new(NamespacedName)
		**out = **in
	}
	if in.Logging != nil {
		in, out := &in.Logging, &out.Logging
		*out = new(EnvoyLogging)
		(*in).DeepCopyInto(*out)
	}
	if in.DefaultHTTPVersions != nil {
		in, out := &in.DefaultHTTPVersions, &out.DefaultHTTPVersions
		*out = make([]HTTPVersionType, len(*in))
		copy(*out, *in)
	}
	if in.Timeouts != nil {
		in, out := &in.Timeouts, &out.Timeouts
		*out = new(TimeoutParameters)
		(*in).DeepCopyInto(*out)
	}
	if in.Cluster != nil {
		in, out := &in.Cluster, &out.Cluster
		*out = new(ClusterParameters)
		(*in).DeepCopyInto(*out)
	}
	if in.Network != nil {
		in, out := &in.Network, &out.Network
		*out = new(NetworkParameters)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoyConfig.
func (in *EnvoyConfig) DeepCopy() *EnvoyConfig {
	if in == nil {
		return nil
	}
	out := new(EnvoyConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoyListener) DeepCopyInto(out *EnvoyListener) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoyListener.
func (in *EnvoyListener) DeepCopy() *EnvoyListener {
	if in == nil {
		return nil
	}
	out := new(EnvoyListener)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoyListenerConfig) DeepCopyInto(out *EnvoyListenerConfig) {
	*out = *in
	if in.UseProxyProto != nil {
		in, out := &in.UseProxyProto, &out.UseProxyProto
		*out = new(bool)
		**out = **in
	}
	if in.DisableAllowChunkedLength != nil {
		in, out := &in.DisableAllowChunkedLength, &out.DisableAllowChunkedLength
		*out = new(bool)
		**out = **in
	}
	if in.DisableMergeSlashes != nil {
		in, out := &in.DisableMergeSlashes, &out.DisableMergeSlashes
		*out = new(bool)
		**out = **in
	}
	if in.MaxRequestsPerConnection != nil {
		in, out := &in.MaxRequestsPerConnection, &out.MaxRequestsPerConnection
		*out = new(uint32)
		**out = **in
	}
	if in.PerConnectionBufferLimitBytes != nil {
		in, out := &in.PerConnectionBufferLimitBytes, &out.PerConnectionBufferLimitBytes
		*out = new(uint32)
		**out = **in
	}
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(EnvoyTLS)
		(*in).DeepCopyInto(*out)
	}
	if in.SocketOptions != nil {
		in, out := &in.SocketOptions, &out.SocketOptions
		*out = new(SocketOptions)
		**out = **in
	}
	if in.MaxRequestsPerIOCycle != nil {
		in, out := &in.MaxRequestsPerIOCycle, &out.MaxRequestsPerIOCycle
		*out = new(uint32)
		**out = **in
	}
	if in.HTTP2MaxConcurrentStreams != nil {
		in, out := &in.HTTP2MaxConcurrentStreams, &out.HTTP2MaxConcurrentStreams
		*out = new(uint32)
		**out = **in
	}
	if in.MaxConnectionsPerListener != nil {
		in, out := &in.MaxConnectionsPerListener, &out.MaxConnectionsPerListener
		*out = new(uint32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoyListenerConfig.
func (in *EnvoyListenerConfig) DeepCopy() *EnvoyListenerConfig {
	if in == nil {
		return nil
	}
	out := new(EnvoyListenerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoyLogging) DeepCopyInto(out *EnvoyLogging) {
	*out = *in
	if in.AccessLogJSONFields != nil {
		in, out := &in.AccessLogJSONFields, &out.AccessLogJSONFields
		*out = make(AccessLogJSONFields, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoyLogging.
func (in *EnvoyLogging) DeepCopy() *EnvoyLogging {
	if in == nil {
		return nil
	}
	out := new(EnvoyLogging)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoySettings) DeepCopyInto(out *EnvoySettings) {
	*out = *in
	if in.NetworkPublishing != nil {
		in, out := &in.NetworkPublishing, &out.NetworkPublishing
		*out = new(NetworkPublishing)
		(*in).DeepCopyInto(*out)
	}
	if in.NodePlacement != nil {
		in, out := &in.NodePlacement, &out.NodePlacement
		*out = new(NodePlacement)
		(*in).DeepCopyInto(*out)
	}
	if in.ExtraVolumes != nil {
		in, out := &in.ExtraVolumes, &out.ExtraVolumes
		*out = make([]corev1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ExtraVolumeMounts != nil {
		in, out := &in.ExtraVolumeMounts, &out.ExtraVolumeMounts
		*out = make([]corev1.VolumeMount, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.PodAnnotations != nil {
		in, out := &in.PodAnnotations, &out.PodAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.DaemonSet != nil {
		in, out := &in.DaemonSet, &out.DaemonSet
		*out = new(DaemonSetSettings)
		(*in).DeepCopyInto(*out)
	}
	if in.Deployment != nil {
		in, out := &in.Deployment, &out.Deployment
		*out = new(DeploymentSettings)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoySettings.
func (in *EnvoySettings) DeepCopy() *EnvoySettings {
	if in == nil {
		return nil
	}
	out := new(EnvoySettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvoyTLS) DeepCopyInto(out *EnvoyTLS) {
	*out = *in
	if in.CipherSuites != nil {
		in, out := &in.CipherSuites, &out.CipherSuites
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvoyTLS.
func (in *EnvoyTLS) DeepCopy() *EnvoyTLS {
	if in == nil {
		return nil
	}
	out := new(EnvoyTLS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExtensionService) DeepCopyInto(out *ExtensionService) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExtensionService.
func (in *ExtensionService) DeepCopy() *ExtensionService {
	if in == nil {
		return nil
	}
	out := new(ExtensionService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ExtensionService) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExtensionServiceList) DeepCopyInto(out *ExtensionServiceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ExtensionService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExtensionServiceList.
func (in *ExtensionServiceList) DeepCopy() *ExtensionServiceList {
	if in == nil {
		return nil
	}
	out := new(ExtensionServiceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ExtensionServiceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExtensionServiceSpec) DeepCopyInto(out *ExtensionServiceSpec) {
	*out = *in
	if in.Services != nil {
		in, out := &in.Services, &out.Services
		*out = make([]ExtensionServiceTarget, len(*in))
		copy(*out, *in)
	}
	if in.UpstreamValidation != nil {
		in, out := &in.UpstreamValidation, &out.UpstreamValidation
		*out = new(v1.UpstreamValidation)
		(*in).DeepCopyInto(*out)
	}
	if in.Protocol != nil {
		in, out := &in.Protocol, &out.Protocol
		*out = new(string)
		**out = **in
	}
	if in.LoadBalancerPolicy != nil {
		in, out := &in.LoadBalancerPolicy, &out.LoadBalancerPolicy
		*out = new(v1.LoadBalancerPolicy)
		(*in).DeepCopyInto(*out)
	}
	if in.TimeoutPolicy != nil {
		in, out := &in.TimeoutPolicy, &out.TimeoutPolicy
		*out = new(v1.TimeoutPolicy)
		**out = **in
	}
	if in.CircuitBreakerPolicy != nil {
		in, out := &in.CircuitBreakerPolicy, &out.CircuitBreakerPolicy
		*out = new(CircuitBreakers)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExtensionServiceSpec.
func (in *ExtensionServiceSpec) DeepCopy() *ExtensionServiceSpec {
	if in == nil {
		return nil
	}
	out := new(ExtensionServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExtensionServiceStatus) DeepCopyInto(out *ExtensionServiceStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.DetailedCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExtensionServiceStatus.
func (in *ExtensionServiceStatus) DeepCopy() *ExtensionServiceStatus {
	if in == nil {
		return nil
	}
	out := new(ExtensionServiceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExtensionServiceTarget) DeepCopyInto(out *ExtensionServiceTarget) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExtensionServiceTarget.
func (in *ExtensionServiceTarget) DeepCopy() *ExtensionServiceTarget {
	if in == nil {
		return nil
	}
	out := new(ExtensionServiceTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in FeatureFlags) DeepCopyInto(out *FeatureFlags) {
	{
		in := &in
		*out = make(FeatureFlags, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FeatureFlags.
func (in FeatureFlags) DeepCopy() FeatureFlags {
	if in == nil {
		return nil
	}
	out := new(FeatureFlags)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayConfig) DeepCopyInto(out *GatewayConfig) {
	*out = *in
	out.GatewayRef = in.GatewayRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayConfig.
func (in *GatewayConfig) DeepCopy() *GatewayConfig {
	if in == nil {
		return nil
	}
	out := new(GatewayConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPProxyConfig) DeepCopyInto(out *HTTPProxyConfig) {
	*out = *in
	if in.DisablePermitInsecure != nil {
		in, out := &in.DisablePermitInsecure, &out.DisablePermitInsecure
		*out = new(bool)
		**out = **in
	}
	if in.RootNamespaces != nil {
		in, out := &in.RootNamespaces, &out.RootNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.FallbackCertificate != nil {
		in, out := &in.FallbackCertificate, &out.FallbackCertificate
		*out = new(NamespacedName)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPProxyConfig.
func (in *HTTPProxyConfig) DeepCopy() *HTTPProxyConfig {
	if in == nil {
		return nil
	}
	out := new(HTTPProxyConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HeadersPolicy) DeepCopyInto(out *HeadersPolicy) {
	*out = *in
	if in.Set != nil {
		in, out := &in.Set, &out.Set
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Remove != nil {
		in, out := &in.Remove, &out.Remove
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HeadersPolicy.
func (in *HeadersPolicy) DeepCopy() *HeadersPolicy {
	if in == nil {
		return nil
	}
	out := new(HeadersPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HealthConfig) DeepCopyInto(out *HealthConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HealthConfig.
func (in *HealthConfig) DeepCopy() *HealthConfig {
	if in == nil {
		return nil
	}
	out := new(HealthConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressConfig) DeepCopyInto(out *IngressConfig) {
	*out = *in
	if in.ClassNames != nil {
		in, out := &in.ClassNames, &out.ClassNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IngressConfig.
func (in *IngressConfig) DeepCopy() *IngressConfig {
	if in == nil {
		return nil
	}
	out := new(IngressConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsConfig) DeepCopyInto(out *MetricsConfig) {
	*out = *in
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(MetricsTLS)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsConfig.
func (in *MetricsConfig) DeepCopy() *MetricsConfig {
	if in == nil {
		return nil
	}
	out := new(MetricsConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsTLS) DeepCopyInto(out *MetricsTLS) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsTLS.
func (in *MetricsTLS) DeepCopy() *MetricsTLS {
	if in == nil {
		return nil
	}
	out := new(MetricsTLS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacedName) DeepCopyInto(out *NamespacedName) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacedName.
func (in *NamespacedName) DeepCopy() *NamespacedName {
	if in == nil {
		return nil
	}
	out := new(NamespacedName)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkParameters) DeepCopyInto(out *NetworkParameters) {
	*out = *in
	if in.XffNumTrustedHops != nil {
		in, out := &in.XffNumTrustedHops, &out.XffNumTrustedHops
		*out = new(uint32)
		**out = **in
	}
	if in.EnvoyAdminPort != nil {
		in, out := &in.EnvoyAdminPort, &out.EnvoyAdminPort
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkParameters.
func (in *NetworkParameters) DeepCopy() *NetworkParameters {
	if in == nil {
		return nil
	}
	out := new(NetworkParameters)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkPublishing) DeepCopyInto(out *NetworkPublishing) {
	*out = *in
	if in.ServiceAnnotations != nil {
		in, out := &in.ServiceAnnotations, &out.ServiceAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkPublishing.
func (in *NetworkPublishing) DeepCopy() *NetworkPublishing {
	if in == nil {
		return nil
	}
	out := new(NetworkPublishing)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodePlacement) DeepCopyInto(out *NodePlacement) {
	*out = *in
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodePlacement.
func (in *NodePlacement) DeepCopy() *NodePlacement {
	if in == nil {
		return nil
	}
	out := new(NodePlacement)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PolicyConfig) DeepCopyInto(out *PolicyConfig) {
	*out = *in
	if in.RequestHeadersPolicy != nil {
		in, out := &in.RequestHeadersPolicy, &out.RequestHeadersPolicy
		*out = new(HeadersPolicy)
		(*in).DeepCopyInto(*out)
	}
	if in.ResponseHeadersPolicy != nil {
		in, out := &in.ResponseHeadersPolicy, &out.ResponseHeadersPolicy
		*out = new(HeadersPolicy)
		(*in).DeepCopyInto(*out)
	}
	if in.ApplyToIngress != nil {
		in, out := &in.ApplyToIngress, &out.ApplyToIngress
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PolicyConfig.
func (in *PolicyConfig) DeepCopy() *PolicyConfig {
	if in == nil {
		return nil
	}
	out := new(PolicyConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RateLimitServiceConfig) DeepCopyInto(out *RateLimitServiceConfig) {
	*out = *in
	out.ExtensionService = in.ExtensionService
	if in.FailOpen != nil {
		in, out := &in.FailOpen, &out.FailOpen
		*out = new(bool)
		**out = **in
	}
	if in.EnableXRateLimitHeaders != nil {
		in, out := &in.EnableXRateLimitHeaders, &out.EnableXRateLimitHeaders
		*out = new(bool)
		**out = **in
	}
	if in.EnableResourceExhaustedCode != nil {
		in, out := &in.EnableResourceExhaustedCode, &out.EnableResourceExhaustedCode
		*out = new(bool)
		**out = **in
	}
	if in.DefaultGlobalRateLimitPolicy != nil {
		in, out := &in.DefaultGlobalRateLimitPolicy, &out.DefaultGlobalRateLimitPolicy
		*out = new(v1.GlobalRateLimitPolicy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RateLimitServiceConfig.
func (in *RateLimitServiceConfig) DeepCopy() *RateLimitServiceConfig {
	if in == nil {
		return nil
	}
	out := new(RateLimitServiceConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SocketOptions) DeepCopyInto(out *SocketOptions) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SocketOptions.
func (in *SocketOptions) DeepCopy() *SocketOptions {
	if in == nil {
		return nil
	}
	out := new(SocketOptions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLS) DeepCopyInto(out *TLS) {
	*out = *in
	if in.Insecure != nil {
		in, out := &in.Insecure, &out.Insecure
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLS.
func (in *TLS) DeepCopy() *TLS {
	if in == nil {
		return nil
	}
	out := new(TLS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TimeoutParameters) DeepCopyInto(out *TimeoutParameters) {
	*out = *in
	if in.RequestTimeout != nil {
		in, out := &in.RequestTimeout, &out.RequestTimeout
		*out = new(string)
		**out = **in
	}
	if in.ConnectionIdleTimeout != nil {
		in, out := &in.ConnectionIdleTimeout, &out.ConnectionIdleTimeout
		*out = new(string)
		**out = **in
	}
	if in.StreamIdleTimeout != nil {
		in, out := &in.StreamIdleTimeout, &out.StreamIdleTimeout
		*out = new(string)
		**out = **in
	}
	if in.MaxConnectionDuration != nil {
		in, out := &in.MaxConnectionDuration, &out.MaxConnectionDuration
		*out = new(string)
		**out = **in
	}
	if in.DelayedCloseTimeout != nil {
		in, out := &in.DelayedCloseTimeout, &out.DelayedCloseTimeout
		*out = new(string)
		**out = **in
	}
	if in.ConnectionShutdownGracePeriod != nil {
		in, out := &in.ConnectionShutdownGracePeriod, &out.ConnectionShutdownGracePeriod
		*out = new(string)
		**out = **in
	}
	if in.ConnectTimeout != nil {
		in, out := &in.ConnectTimeout, &out.ConnectTimeout
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TimeoutParameters.
func (in *TimeoutParameters) DeepCopy() *TimeoutParameters {
	if in == nil {
		return nil
	}
	out := new(TimeoutParameters)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TracingConfig) DeepCopyInto(out *TracingConfig) {
	*out = *in
	if in.IncludePodDetail != nil {
		in, out := &in.IncludePodDetail, &out.IncludePodDetail
		*out = new(bool)
		**out = **in
	}
	if in.ServiceName != nil {
		in, out := &in.ServiceName, &out.ServiceName
		*out = new(string)
		**out = **in
	}
	if in.OverallSampling != nil {
		in, out := &in.OverallSampling, &out.OverallSampling
		*out = new(string)
		**out = **in
	}
	if in.MaxPathTagLength != nil {
		in, out := &in.MaxPathTagLength, &out.MaxPathTagLength
		*out = new(uint32)
		**out = **in
	}
	if in.CustomTags != nil {
		in, out := &in.CustomTags, &out.CustomTags
		*out = make([]*CustomTag, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(CustomTag)
				**out = **in
			}
		}
	}
	if in.ExtensionService != nil {
		in, out := &in.ExtensionService, &out.ExtensionService
		*out = new(NamespacedName)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TracingConfig.
func (in *TracingConfig) DeepCopy() *TracingConfig {
	if in == nil {
		return nil
	}
	out := new(TracingConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *XDSServerConfig) DeepCopyInto(out *XDSServerConfig) {
	*out = *in
	if in.TLS != nil {
		in, out := &in.TLS, &out.TLS
		*out = new(TLS)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new XDSServerConfig.
func (in *XDSServerConfig) DeepCopy() *XDSServerConfig {
	if in == nil {
		return nil
	}
	out := new(XDSServerConfig)
	in.DeepCopyInto(out)
	return out
}
