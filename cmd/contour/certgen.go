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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/projectcontour/contour/internal/certgen"
	"github.com/projectcontour/contour/internal/k8s"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// registercertgen registers the certgen subcommand and flags
// with the Application provided.
func registerCertGen(app *kingpin.Application) (*kingpin.CmdClause, *certgenConfig) {
	var certgenConfig certgenConfig
	certgenApp := app.Command("certgen", "Generate new TLS certs for bootstrapping gRPC over TLS.")

	certgenApp.Flag("kube", "Apply the generated certs directly to the current Kubernetes cluster.").BoolVar(&certgenConfig.OutputKube)
	certgenApp.Flag("yaml", "Render the generated certs as Kubernetes Secrets in YAML form to the current directory.").BoolVar(&certgenConfig.OutputYAML)
	certgenApp.Flag("pem", "Render the generated certs as individual PEM files to the current directory.").BoolVar(&certgenConfig.OutputPEM)
	certgenApp.Flag("incluster", "Use in cluster configuration.").BoolVar(&certgenConfig.InCluster)
	certgenApp.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).StringVar(&certgenConfig.KubeConfig)
	certgenApp.Flag("namespace", "Kubernetes namespace, used for Kube objects.").Default("projectcontour").Envar("CONTOUR_NAMESPACE").StringVar(&certgenConfig.Namespace)
	// NOTE: --certificate-lifetime can be used to accept Duration string once certificate rotation is supported.
	certgenApp.Flag("certificate-lifetime", "Generated certificate lifetime (in days).").Default("365").UintVar(&certgenConfig.Lifetime)
	certgenApp.Flag("overwrite", "Overwrite existing files or Secrets.").BoolVar(&certgenConfig.Overwrite)
	certgenApp.Flag("secrets-format", "Specify how to format the generated Kubernetes Secrets.").Default("legacy").StringVar(&certgenConfig.Format)

	certgenApp.Arg("outputdir", "Directory to write output files into (default \"certs\").").Default("certs").StringVar(&certgenConfig.OutputDir)

	return certgenApp, &certgenConfig
}

// certgenConfig holds the configuration for the certifcate generation process.
type certgenConfig struct {

	// KubeConfig is the path to the Kubeconfig file if we're not running in a cluster
	KubeConfig string

	// Incluster means that we should assume we are running in a Kubernetes cluster and work accordingly.
	InCluster bool

	// Namespace is the namespace to put any generated config into for YAML or Kube outputs.
	Namespace string

	// OutputDir stores the directory where any requested files will be output.
	OutputDir string

	// OutputKube means that the certs generated will be output into a Kubernetes cluster as secrets.
	OutputKube bool

	// OutputYAML means that the certs generated will be output into Kubernetes secrets as YAML in the current directory.
	OutputYAML bool

	// OutputPEM means that the certs generated will be output as PEM files in the current directory.
	OutputPEM bool

	// Lifetime is the number of days for which certificates will be valid.
	Lifetime uint

	// Overwrite allows certgen to overwrite any existing files or Kubernetes Secrets.
	Overwrite bool

	// Format specifies how to format the Kubernetes Secrets (must be "legacy" or "compat").
	Format string
}

// GenerateCerts performs the actual cert generation steps and then returns the certs for the output function.
func GenerateCerts(certConfig *certgenConfig) (map[string][]byte, error) {
	now := time.Now()
	expiry := now.Add(24 * time.Duration(certConfig.Lifetime) * time.Hour)
	caCertPEM, caKeyPEM, err := certgen.NewCA("Project Contour", expiry)
	if err != nil {
		return nil, err
	}

	contourCert, contourKey, err := certgen.NewCert(caCertPEM,
		caKeyPEM,
		expiry,
		"contour",
		certConfig.Namespace,
	)
	if err != nil {
		return nil, err
	}

	envoyCert, envoyKey, err := certgen.NewCert(caCertPEM,
		caKeyPEM,
		expiry,
		"envoy",
		certConfig.Namespace,
	)
	if err != nil {
		return nil, err
	}

	newCerts := make(map[string][]byte)
	newCerts[certgen.CACertificateKey] = caCertPEM
	newCerts[certgen.ContourCertificateKey] = contourCert
	newCerts[certgen.ContourPrivateKeyKey] = contourKey
	newCerts[certgen.EnvoyCertificateKey] = envoyCert
	newCerts[certgen.EnvoyPrivateKeyKey] = envoyKey

	return newCerts, nil

}

// OutputCerts outputs the certs in certs as directed by config.
func OutputCerts(config *certgenConfig, kubeclient *kubernetes.Clientset, certs map[string][]byte) {
	secrets := []*corev1.Secret{}
	force := certgen.NoOverwrite
	if config.Overwrite {
		force = certgen.Overwrite
	}

	if config.OutputYAML || config.OutputKube {
		switch config.Format {
		case "legacy":
			secrets = certgen.AsLegacySecrets(config.Namespace, certs)
		case "compact":
			secrets = certgen.AsSecrets(config.Namespace, certs)
		default:
			check(fmt.Errorf("unsupported Secrets format %q", config.Format))
		}
	}

	if config.OutputPEM {
		fmt.Printf("Writing certificates to PEM files in %s/\n", config.OutputDir)
		check(certgen.WriteCertsPEM(config.OutputDir, certs, force))
	}

	if config.OutputYAML {
		fmt.Printf("Writing %q format Secrets to YAML files in %s/\n", config.Format, config.OutputDir)
		check(certgen.WriteSecretsYAML(config.OutputDir, secrets, force))
	}

	if config.OutputKube {
		fmt.Printf("Writing %q format Secrets to namespace %q\n", config.Format, config.Namespace)
		check(certgen.WriteSecretsKube(kubeclient, secrets, force))
	}
}

func doCertgen(config *certgenConfig) {
	generatedCerts, err := GenerateCerts(config)
	check(err)

	clients, err := k8s.NewClients(config.KubeConfig, config.InCluster)
	if err != nil {
		check(fmt.Errorf("failed to create Kubernetes client: %w", err))
	}

	OutputCerts(config, clients.ClientSet(), generatedCerts)
}
