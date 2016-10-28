#!/usr/bin/env bash

set -e
echo "" > coverage.txt

go list -f '"go test -v -race -covermode=atomic -coverprofile={{.Name}}.coverprofile -coverpkg={{range $i, $f := .XTestImports}}{{if eq (printf "%.24s" $f) "github.com/dnaeon/go-vcr" }}{{$f}},{{end}}{{end}}{{.ImportPath}} {{.ImportPath}}"' ./... | grep -v vendor |  xargs -I {} bash -c {}

gover . coverage.txt

rm *.coverprofile
