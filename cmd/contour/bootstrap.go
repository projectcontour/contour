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

package main

import (
	"github.com/alecthomas/kingpin/v2"

	"github.com/projectcontour/contour/internal/envoy"
)

// registerBootstrap registers the bootstrap subcommand and flags
// with the Application provided.
func registerBootstrap(app *kingpin.Application) (*kingpin.CmdClause, *envoy.BootstrapConfig) {
	var config envoy.BootstrapConfig

	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")
	bootstrap.Arg("path", "Configuration file ('-' for standard output).").Required().StringVar(&config.Path)

	bootstrap.Flag("admin-address", "Path to Envoy admin unix domain socket.").Default("/admin/admin.sock").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "DEPRECATED: Envoy admin interface port.").IntVar(&config.AdminPort)
	bootstrap.Flag("dns-lookup-family", "Defines what DNS Resolution Policy to use for Envoy -> Contour cluster name lookup. Either v4, v6, auto, or all.").StringVar(&config.DNSLookupFamily)
	bootstrap.Flag("envoy-cafile", "CA Filename for Envoy secure xDS gRPC communication.").Envar("ENVOY_CAFILE").StringVar(&config.GrpcCABundle)
	bootstrap.Flag("envoy-cert-file", "Client certificate filename for Envoy secure xDS gRPC communication.").Envar("ENVOY_CERT_FILE").StringVar(&config.GrpcClientCert)
	bootstrap.Flag("envoy-key-file", "Client key filename for Envoy secure xDS gRPC communication.").Envar("ENVOY_KEY_FILE").StringVar(&config.GrpcClientKey)
	bootstrap.Flag("namespace", "The namespace the Envoy container will run in.").Envar("CONTOUR_NAMESPACE").Default("projectcontour").StringVar(&config.Namespace)
	bootstrap.Flag("overload-max-heap", "Defines the maximum heap size in bytes until overload manager stops accepting new connections.").Uint64Var(&config.MaximumHeapSizeBytes)
	bootstrap.Flag("resources-dir", "Directory where configuration files will be written to.").StringVar(&config.ResourcesDir)
	bootstrap.Flag("xds-address", "xDS gRPC API address.").StringVar(&config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port.").IntVar(&config.XDSGRPCPort)
	bootstrap.Flag("xds-resource-version", "The versions of the xDS resources to request from Contour.").Default("v3").StringVar((*string)(&config.XDSResourceVersion))

	return bootstrap, &config
}
