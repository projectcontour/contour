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

	var visit func(dag.Vertex)
	visit = func(parent dag.Vertex) {
		var route *dag.Route
		switch parent := parent.(type) {
		case *dag.Secret:
			fmt.Fprintf(w, `"%p" [shape=record, label="{secret|%s/%s}"]`+"\n", parent, parent.Namespace(), parent.Name())
		case *dag.Service:
			fmt.Fprintf(w, `"%p" [shape=record, label="{service|%s/%s}"]`+"\n", parent, parent.Namespace(), parent.Name())
		case *dag.VirtualHost:
			fmt.Fprintf(w, `"%p" [shape=record, label="{host|%s}"]`+"\n", parent, parent.FQDN())
		case *dag.Route:
			route = parent
			fmt.Fprintf(w, `"%p" [shape=record, label="{prefix|%s}"]`+"\n", parent, parent.Prefix())
		}
		parent.Visit(func(child dag.Vertex) {
			visit(child)
			switch child := child.(type) {
			default:
				fmt.Fprintf(w, `"%p" -> "%p"`+"\n", parent, child)
			case *dag.Service:
				fmt.Fprintf(w, `"%p" -> "%p" [label="port: %s"]`+"\n", parent, child, route.ServicePort())
			}
		})
	}

	dw.DAG.Visit(visit)

	fmt.Fprintln(w, "}")
}
