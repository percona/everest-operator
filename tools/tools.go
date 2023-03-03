//go:build tools

package tools

import (
	_ "github.com/BurntSushi/go-sumtype"
	_ "github.com/apache/skywalking-eyes/cmd/license-eye"
	_ "github.com/daixiang0/gci"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/reviewdog/reviewdog/cmd/reviewdog"
	_ "golang.org/x/tools/cmd/goimports"
	_ "mvdan.cc/gofumpt"
)

//go:generate go build -o ../bin/gofumpt mvdan.cc/gofumpt
//go:generate go build -o ../bin/goimports golang.org/x/tools/cmd/goimports
//go:generate go build -o ../bin/gci github.com/daixiang0/gci
//go:generate env CGO_ENABLED=0 go build -o ../bin/license-eye github.com/apache/skywalking-eyes/cmd/license-eye
//go:generate go build -o ../bin/go-sumtype github.com/BurntSushi/go-sumtype
//go:generate go build -o ../bin/reviewdog github.com/reviewdog/reviewdog/cmd/reviewdog
//go:generate go build -o ../bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint
