//go:build tools

package tools

import (
	// nolint:typecheck
	_ "github.com/ahmetb/gen-crd-api-reference-docs"
	// nolint:typecheck
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	// nolint:typecheck
	_ "github.com/vektra/mockery/v2"
	// nolint:typecheck
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kustomize/kyaml"
)
