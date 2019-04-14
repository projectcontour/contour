// +build tools

package tools

import (
	_ "mvdan.cc/unparam"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/gordonklaus/ineffassign"
	_ "github.com/mdempsky/unconvert"
	_ "github.com/kisielk/errcheck"

	_ "k8s.io/code-generator/cmd/client-gen"
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	_ "k8s.io/code-generator/cmd/defaulter-gen"
	_ "k8s.io/code-generator/cmd/lister-gen"
	_ "k8s.io/code-generator/cmd/informer-gen"
)
