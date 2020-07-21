package k8s

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/types"
)

func TestNamespacedNameFrom(t *testing.T) {
	run := func(testName string, got types.NamespacedName, want types.NamespacedName) {
		t.Helper()
		t.Run(testName, func(t *testing.T) {
			t.Helper()
			opts := []cmp.Option{
				cmp.AllowUnexported(types.NamespacedName{}),
			}
			if diff := cmp.Diff(want, got, opts...); diff != "" {
				t.Fatal(diff)
			}
		})
	}

	run("no namespace",
		NamespacedNameFrom("secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "defns",
		},
	)

	run("with namespace",
		NamespacedNameFrom("ns1/secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "ns1",
		},
	)

	run("missing namespace",
		NamespacedNameFrom("/secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "defns",
		},
	)

	run("missing secret name",
		NamespacedNameFrom("secret/", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "",
			Namespace: "secret",
		},
	)

	run("missing default namespace",
		NamespacedNameFrom("secret"),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "default",
		},
	)

	run("empty resource",
		NamespacedNameFrom(""),
		types.NamespacedName{
			Name:      "",
			Namespace: "default",
		},
	)
}
