// Copyright © 2019 VMware
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
	"io"
	"os"

	"github.com/golang/protobuf/jsonpb"
	"github.com/projectcontour/contour/internal/envoy"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registerBootstrap registers the bootstrap subcommand and flags
// with the Application provided.
func registerBootstrap(app *kingpin.Application) (*kingpin.CmdClause, *bootstrapContext) {
	var ctx bootstrapContext

	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")
	bootstrap.Arg("path", "Configuration file ('-' for standard output)").Required().StringVar(&ctx.path)
	bootstrap.Flag("admin-address", "Envoy admin interface address").StringVar(&ctx.config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port").IntVar(&ctx.config.AdminPort)
	bootstrap.Flag("xds-address", "xDS gRPC API address").StringVar(&ctx.config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port").IntVar(&ctx.config.XDSGRPCPort)
	bootstrap.Flag("envoy-cafile", "gRPC CA Filename for Envoy to load").Envar("ENVOY_CAFILE").StringVar(&ctx.config.GrpcCABundle)
	bootstrap.Flag("envoy-cert-file", "gRPC Client cert filename for Envoy to load").Envar("ENVOY_CERT_FILE").StringVar(&ctx.config.GrpcClientCert)
	bootstrap.Flag("envoy-key-file", "gRPC Client key filename for Envoy to load").Envar("ENVOY_KEY_FILE").StringVar(&ctx.config.GrpcClientKey)
	bootstrap.Flag("namespace", "The namespace the Envoy container will run in").Envar("CONTOUR_NAMESPACE").Default("projectcontour").StringVar(&ctx.config.Namespace)
	return bootstrap, &ctx
}

type bootstrapContext struct {
	config envoy.BootstrapConfig
	path   string
}

// doBootstrap writes an Envoy bootstrap configuration file to the supplied path.
func doBootstrap(ctx *bootstrapContext) {
	var out io.Writer

	switch ctx.path {
	case "-":
		out = os.Stdout
	default:
		f, err := os.Create(ctx.path)
		check(err)

		out = f

		defer func() {
			check(f.Close())
		}()
	}

	m := &jsonpb.Marshaler{OrigName: true}

	check(m.Marshal(out, envoy.Bootstrap(&ctx.config)))
}
