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
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

func TestServeContextIngressRouteRootNamespaces(t *testing.T) {
	tests := map[string]struct {
		ctx  serveContext
		want []string
	}{
		"empty": {
			ctx: serveContext{
				rootNamespaces: "",
			},
			want: nil,
		},
		"blank-ish": {
			ctx: serveContext{
				rootNamespaces: " \t ",
			},
			want: nil,
		},
		"one value": {
			ctx: serveContext{
				rootNamespaces: "projectcontour",
			},
			want: []string{"projectcontour"},
		},
		"multiple, easy": {
			ctx: serveContext{
				rootNamespaces: "prod1,prod2,prod3",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
		"multiple, hard": {
			ctx: serveContext{
				rootNamespaces: "prod1, prod2, prod3 ",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.ctx.ingressRouteRootNamespaces()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected: %q, got: %q", tc.want, got)
			}
		})
	}
}

func TestServeContextTLSParams(t *testing.T) {
	tests := map[string]struct {
		ctx         serveContext
		expecterror bool
	}{
		"tls supplied correctly": {
			ctx: serveContext{
				caFile:      "cacert.pem",
				contourCert: "contourcert.pem",
				contourKey:  "contourkey.pem",
			},
			expecterror: false,
		},
		"tls partially supplied": {
			ctx: serveContext{
				contourCert: "contourcert.pem",
				contourKey:  "contourkey.pem",
			},
			expecterror: true,
		},
		"tls not supplied": {
			ctx:         serveContext{},
			expecterror: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.ctx.verifyTLSFlags()
			goterror := err != nil
			if goterror != tc.expecterror {
				t.Errorf("TLS Config: %s", err)
			}
		})
	}
}

func TestConfigFileDefaultOverrideImport(t *testing.T) {
	tests := map[string]struct {
		yamlIn string
		want   func() *serveContext
	}{
		"empty configuration": {
			yamlIn: ``,
			want:   newServeContext,
		},
		"defaults in yaml": {
			yamlIn: `
incluster: false
disablePermitInsecure: false
leaderelection:
  configmap-name: leader-elect
  configmap-namespace: projectcontour
  lease-duration: 15s
  renew-deadline: 10s
  retry-period: 2s
`,
			want: newServeContext,
		},
		"blank tls configuration": {
			yamlIn: `
tls:
`,
			want: newServeContext,
		},
		"tls configuration only": {
			yamlIn: `
tls:
  minimum-protocol-version: 1.2
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.TLSConfig.MinimumProtocolVersion = "1.2"
				return ctx
			},
		},
		"leader election namespace and configmap only": {
			yamlIn: `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.LeaderElectionConfig.Name = "foo"
				ctx.LeaderElectionConfig.Namespace = "bar"
				return ctx
			},
		},
		"leader election all fields set": {
			yamlIn: `
leaderelection:
  configmap-name: foo
  configmap-namespace: bar
  lease-duration: 600s
  renew-deadline: 500s
  retry-period: 60s
`,
			want: func() *serveContext {
				ctx := newServeContext()
				ctx.LeaderElectionConfig.Name = "foo"
				ctx.LeaderElectionConfig.Namespace = "bar"
				ctx.LeaderElectionConfig.LeaseDuration = 600 * time.Second
				ctx.LeaderElectionConfig.RenewDeadline = 500 * time.Second
				ctx.LeaderElectionConfig.RetryPeriod = 60 * time.Second
				return ctx
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := newServeContext()
			err := yaml.Unmarshal([]byte(tc.yamlIn), got)
			checkErr(t, err)
			want := tc.want()

			if diff := cmp.Diff(*want, *got, cmp.AllowUnexported(serveContext{})); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}
