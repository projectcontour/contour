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

package debug

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug/mocks"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

//go:generate go run github.com/vektra/mockery/v2 --case=snake --name=DagBuilder --srcpkg=github.com/projectcontour/contour/internal/debug  --disable-version-string

func TestWriteDotEscapesLabels(t *testing.T) {
	d := dag.DAG{
		Listeners: map[string]*dag.Listener{},
	}
	for _, l := range getTestListeners() {
		d.Listeners[l.Name] = l
	}
	b := mocks.DagBuilder{}
	b.On("Build").Return(&d)

	dw := &dotWriter{
		Builder: &b,
	}
	buf := bytes.Buffer{}
	dw.writeDot(&buf)

	require.NotNil(t, buf)
	var line string
	var err error
	labelMatcher := regexp.MustCompile(`label="(.*)"`)
	for ; err == nil || len(line) > 0; line, err = buf.ReadString('\n') {
		if match := labelMatcher.FindStringSubmatch(line); match != nil {
			require.NotContains(t, match[1], `"`, "Unescaped quote")
			require.NotContains(t, match[1], `<`, "Unescaped less than")
			require.NotContains(t, match[1], `>`, "Unescaped greater than")
		}
	}
}

// TestWriteDotLineCount is a pinning test to sanity check during refactor.
func TestWriteDotLineCount(t *testing.T) {
	d := dag.DAG{
		Listeners: map[string]*dag.Listener{},
	}
	for _, l := range getTestListeners() {
		d.Listeners[l.Name] = l
	}
	b := mocks.DagBuilder{}
	b.On("Build").Return(&d)

	dw := &dotWriter{
		Builder: &b,
	}
	buf := bytes.Buffer{}
	dw.writeDot(&buf)

	require.NotNil(t, buf)
	var line string
	var err error
	lineCount := 0
	labeledLineCount := 0
	labelMatcher := regexp.MustCompile(`label="(.*)"`)
	for ; err == nil || len(line) > 0; line, err = buf.ReadString('\n') {
		lineCount++
		if match := labelMatcher.FindStringSubmatch(line); match != nil {
			labeledLineCount++
		}
	}
	require.EqualValues(t, 21, lineCount)
	require.EqualValues(t, 9, labeledLineCount)
}

func getTestListeners() []*dag.Listener {
	vh1 := dag.VirtualHost{
		Name: "test.projectcontour.io",
	}
	vh1.AddRoute(newPrefixRoute("/", newTestService()))

	vh2 := dag.VirtualHost{
		Name: "another.projectcontour.io",
	}
	vh2.AddRoute(newPrefixRoute(`/"<>"`, newTestService()))

	l := dag.Listener{
		Name: dag.HTTP_LISTENER_NAME,
		Port: 80,
		VirtualHosts: []*dag.VirtualHost{
			&vh1, &vh2,
		},
	}

	return []*dag.Listener{&l}
}

func newPrefixRoute(prefix string, svc *dag.Service) *dag.Route {
	return &dag.Route{
		PathMatchCondition: &dag.PrefixMatchCondition{Prefix: prefix, PrefixMatchType: dag.PrefixMatchString},
		Clusters: []*dag.Cluster{{
			Upstream: svc,
			Protocol: svc.Protocol,
			Weight:   svc.Weighted.Weight,
		}},
	}
}

func newTestService() *dag.Service {
	return &dag.Service{
		Weighted: dag.WeightedService{
			Weight:           1,
			ServiceName:      "testService",
			ServiceNamespace: "projectcontour",
			ServicePort: v1.ServicePort{
				Name:       "http",
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			},
		},
	}
}
