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
	"github.com/projectcontour/contour/internal/envoy"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registerBootstrap registers the bootstrap subcommand and flags
// with the Application provided.
func registerBootstrap(app *kingpin.Application) (*kingpin.CmdClause, *envoy.BootstrapConfig) {
	var config envoy.BootstrapConfig

	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")
	bootstrap.Arg("path", "Configuration file ('-' for standard output).").Required().StringVar(&config.Path)
	bootstrap.Flag("resources-dir", "Directory where configuration files will be written to.").StringVar(&config.ResourcesDir)
	bootstrap.Flag("admin-address", "Envoy admin interface address.").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port.").IntVar(&config.AdminPort)
	bootstrap.Flag("xds-address", "xDS gRPC API address.").StringVar(&config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port.").IntVar(&config.XDSGRPCPort)
	bootstrap.Flag("envoy-cafile", "gRPC CA Filename for Envoy to load.").Envar("ENVOY_CAFILE").StringVar(&config.GrpcCABundle)
	bootstrap.Flag("envoy-cert-file", "gRPC Client cert filename for Envoy to load.").Envar("ENVOY_CERT_FILE").StringVar(&config.GrpcClientCert)
	bootstrap.Flag("envoy-key-file", "gRPC Client key filename for Envoy to load.").Envar("ENVOY_KEY_FILE").StringVar(&config.GrpcClientKey)
	bootstrap.Flag("namespace", "The namespace the Envoy container will run in.").Envar("CONTOUR_NAMESPACE").Default("projectcontour").StringVar(&config.Namespace)
	return bootstrap, &config
}
