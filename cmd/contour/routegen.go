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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/sirupsen/logrus"

	contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/routegen"
	v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/projectcontour/contour/pkg/config"
)

// DefaultTimeout for DAG processing.
const defaultTimeout = 10 * time.Second

type bufioFileCloser struct {
	file *os.File
	*bufio.Writer
}

func (bfc *bufioFileCloser) Close() error {
	if err := bfc.Flush(); err != nil {
		return err
	}
	return bfc.file.Close()
}

// routeGenConfig holds configuration for the route generation command.
type routeGenConfig struct {
	inputManifests []string
	serveCtx       *serveContext
	output         string
}

// newRouteGenConfig creates a new instance of routeGenConfig with default settings.
func newRouteGenConfig() *routeGenConfig {
	return &routeGenConfig{
		serveCtx: newServeContext(),
	}
}

// registerRouteGen registers the routegen subcommand and associated flags with the provided kingpin.Application.
func registerRouteGen(app *kingpin.Application) (*kingpin.CmdClause, *routeGenConfig) {
	cfg := newRouteGenConfig()

	var configFile string
	var parsed bool

	parseContourConfigFile := func(_ *kingpin.ParseContext) error {
		if cfg.serveCtx.contourConfigurationName != "" && configFile != "" {
			return fmt.Errorf("cannot specify both --contour-config and -c/--config-path")
		}

		if parsed || configFile == "" {
			return nil
		}

		file, err := os.Open(configFile)
		if err != nil {
			return err
		}
		defer file.Close()

		params, err := config.Parse(file)
		if err != nil {
			return err
		}

		if err := params.Validate(); err != nil {
			return fmt.Errorf("invalid Contour configuration: %w", err)
		}

		parsed = true
		cfg.serveCtx.Config = *params
		return nil
	}

	routeGenCmd := app.Command("routegen", "Generate Envoy route configuration based on server config and resources.")
	routeGenCmd.Arg("resources", "Set of input resource manifests for the Envoy route configuration.").Required().StringsVar(&cfg.inputManifests)
	routeGenCmd.Flag("config-path", "Path to base configuration file.").Short('c').PlaceHolder("/path/to/file").Action(parseContourConfigFile).ExistingFileVar(&configFile)
	routeGenCmd.Flag("ingress-class-name", "Contour IngressClass name.").PlaceHolder("<name>").StringVar(&cfg.serveCtx.ingressClassName)
	routeGenCmd.Flag("output", "File to write route config into (defaults to stdout).").StringVar(&cfg.output)

	return routeGenCmd, cfg
}

// doRouteGen processes the route generation using the provided configuration and logger.
func doRouteGen(cfg *routeGenConfig, log *logrus.Logger) {
	log.Infof("Generating Envoy routes for resources defined in %q", cfg.inputManifests)

	writer, err := fileOutputWriter(cfg.output)
	if err != nil {
		log.Errorf("Could not open file %v: %v", cfg.output, err)
		return
	}
	defer writer.Close()

	resources, err := routegen.ReadManifestFiles(cfg.inputManifests, log)
	if err != nil {
		log.Errorf("Could not read input manifests: %v", err)
		return
	}

	dagBuilder, err := computeDagBuilder(cfg, log)
	if err != nil {
		log.Errorf("Could not initialize DAG builder: %v", err)
		return
	}

	routeGenerator := routegen.NewRouteGenerator(dagBuilder, &v3.RouteCache{})
	rawEnvoyRouteConfigs := routeGenerator.Run(resources)

	if err := json.NewEncoder(writer).Encode(rawEnvoyRouteConfigs[0]); err != nil {
		log.Errorf("Failed to write Envoy routes: %v", err)
	}
}

// fileOutputWriter creates an io.WriteCloser for the specified output. If the output is empty, it defaults to stdout.
func fileOutputWriter(out string) (io.WriteCloser, error) {
	if out == "" {
		return os.Stdout, nil
	}

	path := filepath.Dir(out)
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create directories for %s: %w", out, err)
	}

	file, err := os.Create(out)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", out, err)
	}

	return &bufioFileCloser{
		file:   file,
		Writer: bufio.NewWriter(file),
	}, nil
}

// computeDagBuilder initializes a dag.Builder based on the configuration provided.
func computeDagBuilder(cfg *routeGenConfig, log logrus.FieldLogger) (*dag.Builder, error) {
	providedConfig := cfg.serveCtx.convertToContourConfigurationSpec()

	contourConfiguration, err := contourconfig.OverlayOnDefaults(providedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to overlay configuration on defaults: %w", err)
	}

	if err := contourConfiguration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Contour configuration: %w", err)
	}

	return dagBuilderFromContourConfig(contourConfiguration, log), nil
}

// dagBuilderFromContourConfig creates a dag.Builder from the given Contour configuration and logger.
func dagBuilderFromContourConfig(config contour_v1alpha1.ContourConfigurationSpec, log logrus.FieldLogger) *dag.Builder {
	var ingressClassNames []string
	if config.Ingress != nil {
		ingressClassNames = config.Ingress.ClassNames
	}

	dagProcessors := []dag.Processor{
		&dag.ListenerProcessor{
			HTTPAddress:  config.Envoy.HTTPListener.Address,
			HTTPPort:     config.Envoy.HTTPListener.Port,
			HTTPSAddress: config.Envoy.HTTPSListener.Address,
			HTTPSPort:    config.Envoy.HTTPSListener.Port,
		},
		&dag.ExtensionServiceProcessor{
			FieldLogger:       log.WithField("context", "ExtensionServiceProcessor"),
			ClientCertificate: nil,
			ConnectTimeout:    defaultTimeout,
		},
		&dag.IngressProcessor{
			EnableExternalNameService:     *config.EnableExternalNameService,
			ConnectTimeout:                defaultTimeout,
			MaxRequestsPerConnection:      config.Envoy.Cluster.MaxRequestsPerConnection,
			PerConnectionBufferLimitBytes: config.Envoy.Cluster.PerConnectionBufferLimitBytes,
			SetSourceMetadataOnRoutes:     false,
		},
		&dag.GatewayAPIProcessor{
			EnableExternalNameService:     *config.EnableExternalNameService,
			ConnectTimeout:                defaultTimeout,
			MaxRequestsPerConnection:      config.Envoy.Cluster.MaxRequestsPerConnection,
			PerConnectionBufferLimitBytes: config.Envoy.Cluster.PerConnectionBufferLimitBytes,
			SetSourceMetadataOnRoutes:     false,
		},
		&dag.HTTPProxyProcessor{
			EnableExternalNameService:     *config.EnableExternalNameService,
			DisablePermitInsecure:         *config.HTTPProxy.DisablePermitInsecure,
			DNSLookupFamily:               config.Envoy.Cluster.DNSLookupFamily,
			ConnectTimeout:                defaultTimeout,
			GlobalExternalAuthorization:   config.GlobalExternalAuthorization,
			MaxRequestsPerConnection:      config.Envoy.Cluster.MaxRequestsPerConnection,
			GlobalRateLimitService:        config.RateLimitService,
			PerConnectionBufferLimitBytes: config.Envoy.Cluster.PerConnectionBufferLimitBytes,
			SetSourceMetadataOnRoutes:     false,
		},
	}

	return &dag.Builder{
		Source: dag.KubernetesCache{
			RootNamespaces:           config.HTTPProxy.RootNamespaces,
			IngressClassNames:        ingressClassNames,
			ConfiguredGatewayToCache: nil,
			ConfiguredSecretRefs:     nil,
			FieldLogger:              log.WithField("context", "KubernetesCache"),
			Client:                   nil,
			Metrics:                  nil,
		},
		Processors: dagProcessors,
		Metrics:    nil,
		Config: dag.Config{
			UseReadableClusterNames: true,
		},
	}
}
