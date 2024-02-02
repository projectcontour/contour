package tools

import (
	// nolint:typecheck
	_ "github.com/ahmetb/gen-crd-api-reference-docs"

	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"

	_ "mvdan.cc/gofumpt"

	// nolint:typecheck
	_ "github.com/vektra/mockery/v2"

	// nolint:typecheck
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"

	_ "sigs.k8s.io/kustomize/kyaml"
)
