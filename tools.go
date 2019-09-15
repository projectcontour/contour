// +build tools

package tools

import (
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/gordonklaus/ineffassign"
	_ "github.com/kisielk/errcheck"
	_ "github.com/mdempsky/unconvert"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "mvdan.cc/unparam"

	_ "k8s.io/code-generator/cmd/client-gen"
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	_ "k8s.io/code-generator/cmd/defaulter-gen"
	_ "k8s.io/code-generator/cmd/informer-gen"
	_ "k8s.io/code-generator/cmd/lister-gen"
)
