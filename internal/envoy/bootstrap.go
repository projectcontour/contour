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
//
// Package envoy contains APIs for translating between Contour
// objects and Envoy configuration APIs and types.

package envoy

import (
	"os"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/pkg/config"
)

// SDSResourcesSubdirectory stores the subdirectory name where SDS path resources are stored to.
const SDSResourcesSubdirectory = "sds"

// SDSTLSCertificateFile stores the path to the SDS resource with Envoy's
// client certificate and key for XDS gRPC connection.
const SDSTLSCertificateFile = "xds-tls-certificate.json"

// SDSValidationContextFile stores the path to the SDS resource with
// CA certificates for Envoy to use for the XDS gRPC connection.
const SDSValidationContextFile = "xds-validation-context.json"

// BootstrapConfig holds configuration values for a Bootstrap configuration.
type BootstrapConfig struct {
	// AdminAccessLogPath is the path to write the access log for the administration server.
	// Defaults to /dev/null.
	AdminAccessLogPath string

	// AdminAddress is the TCP address that the administration server will listen on.
	// Defaults to 127.0.0.1.
	AdminAddress string

	// AdminPort is the port that the administration server will listen on.
	// Defaults to 9001.
	AdminPort int

	// XDSAddress is the TCP address of the gRPC XDS management server.
	// Defaults to 127.0.0.1.
	XDSAddress string

	// XDSGRPCPort is the management server port that provides the v2 gRPC API.
	// Defaults to 8001.
	XDSGRPCPort int

	// XDSResourceVersion defines the XDS Server Version to use.
	// Defaults to "v2"
	XDSResourceVersion config.ResourceVersion

	// Namespace is the namespace where Contour is running
	Namespace string

	// GrpcCABundle is the filename that contains a CA certificate chain that can
	// verify the client cert.
	GrpcCABundle string

	// GrpcClientCert is the filename that contains a client certificate. May contain a full bundle if you
	// don't want to pass a CA Bundle.
	GrpcClientCert string

	// GrpcClientKey is the filename that contains a client key for secure gRPC with TLS.
	GrpcClientKey string

	// Path is the filename for the bootstrap configuration file to be created.
	Path string

	// ResourcesDir is the directory where out of line Envoy resources can be placed.
	ResourcesDir string

	// SkipFilePathCheck specifies whether to skip checking whether files
	// referenced in the configuration actually exist. This option is for
	// testing only.
	SkipFilePathCheck bool
}

func (c *BootstrapConfig) GetXdsAddress() string { return stringOrDefault(c.XDSAddress, "127.0.0.1") }
func (c *BootstrapConfig) GetXdsGRPCPort() int   { return intOrDefault(c.XDSGRPCPort, 8001) }
func (c *BootstrapConfig) GetAdminAddress() string {
	return stringOrDefault(c.AdminAddress, "127.0.0.1")
}
func (c *BootstrapConfig) GetAdminPort() int { return intOrDefault(c.AdminPort, 9001) }
func (c *BootstrapConfig) GetAdminAccessLogPath() string {
	return stringOrDefault(c.AdminAccessLogPath, "/dev/null")
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intOrDefault(i, def int) int {
	if i == 0 {
		return def
	}
	return i
}

func WriteConfig(filename string, config proto.Message) (err error) {
	var out *os.File

	if filename == "-" {
		out = os.Stdout
	} else {
		out, err = os.Create(filename)
		if err != nil {
			return
		}
		defer func() {
			err = out.Close()
		}()
	}

	m := &jsonpb.Marshaler{OrigName: true}
	return m.Marshal(out, config)
}
