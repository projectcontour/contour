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

package debug

import (
	"fmt"
	"io"

	"github.com/heptio/contour/internal/dag"
	"github.com/sirupsen/logrus"
)

// quick and dirty dot debugging package

type dotWriter struct {
	*dag.DAG
	logrus.FieldLogger
}

func (dw *dotWriter) writeDot(w io.Writer) {
	fmt.Fprintln(w, "digraph DAG {\nrankdir=\"LR\"")

	var vhost string
	var visit func(dag.Vertex)
	visit = func(v dag.Vertex) {
		var name string
		switch v := v.(type) {
		case *dag.Secret:
			name = fmt.Sprintf("secret/%s/%s", v.Namespace(), v.Name())
			fmt.Fprintf(w, "%q [shape=record, label=\"{secret|%s}\"]\n", name, name[len("secret/"):])
		case *dag.Service:
			name = fmt.Sprintf("service/%s/%s", v.Namespace(), v.Name())
			fmt.Fprintf(w, "%q [shape=record, label=\"{service|%s}\"]\n", name, name[len("service/"):])
		case *dag.VirtualHost:
			vhost = fmt.Sprintf("virtualhost/%s", v.FQDN())
			name = vhost
			fmt.Fprintf(w, "%q [shape=record, label=\"{host|%s}\"]\n", name, v.FQDN())
		case *dag.Route:
			name = fmt.Sprintf("%s/path/%s", vhost, v.Prefix())
			fmt.Fprintf(w, "%q [shape=record, label=\"{prefix|%s}\"]\n", name, v.Prefix())
		}
		v.ChildVertices(func(v dag.Vertex) {
			visit(v)
			switch v := v.(type) {
			case *dag.Secret:
				fmt.Fprintf(w, "%q -> \"secret/%s/%s\"\n", name, v.Namespace(), v.Name())
			case *dag.Service:
				fmt.Fprintf(w, "%q -> \"service/%s/%s\"\n", name, v.Namespace(), v.Name())
			case *dag.VirtualHost:
				fmt.Fprintf(w, "%q -> \"virtualhost/%s\"\n", name, v.FQDN())
			case *dag.Route:
				fmt.Fprintf(w, "%q -> \"%s/path/%s\"\n", vhost, name, v.Prefix())
			}
		})
	}

	dw.DAG.Roots(visit)

	fmt.Fprintln(w, "}")
}
