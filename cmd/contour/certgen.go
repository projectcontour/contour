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

package main

import (
	"os"
	"path/filepath"

	"github.com/heptio/contour/internal/certgen"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registercertgen registers the certgen subcommand and flags
// with the Application provided.
func registerCertGen(app *kingpin.Application) (*kingpin.CmdClause, *certgen.Config) {
	var certgenConfig certgen.Config
	certgenApp := app.Command("certgen", "Generate new TLS certs for bootstrapping gRPC over TLS")
	certgenApp.Flag("kube", "Apply the generated certs directly to the current Kubernetes cluster").BoolVar(&certgenConfig.OutputKube)
	certgenApp.Flag("yaml", "Render the generated certs as Kubernetes Secrets in YAML form to the current directory").BoolVar(&certgenConfig.OutputYAML)
	certgenApp.Flag("pem", "Render the generated certs as individual PEM files to the current directory").BoolVar(&certgenConfig.OutputPEM)
	certgenApp.Flag("incluster", "use in cluster configuration.").BoolVar(&certgenConfig.InCluster)
	certgenApp.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).StringVar(&certgenConfig.KubeConfig)
	certgenApp.Flag("namespace", "Kubernetes namespace, used for Kube objects").Default("heptio-contour").Envar("CONTOUR_NAMESPACE").StringVar(&certgenConfig.Namespace)
	certgenApp.Arg("outputdir", "Directory to output any files to").Default("certs").StringVar(&certgenConfig.OutputDir)

	return certgenApp, &certgenConfig
}
