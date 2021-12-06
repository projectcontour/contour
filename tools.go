//go:build tools
// +build tools

package tools

import (
	_ "github.com/ahmetb/gen-crd-api-reference-docs"
	_ "github.com/vektra/mockery/v2"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kustomize/kyaml"
)
