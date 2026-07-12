// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package parser

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
)

type parseOptions struct {
	AllErrors         bool
	ParseComments     bool
	Trace             bool
	DeclarationErrors bool
}

func defaultParseOptions() parseOptions {
	return parseOptions{
		AllErrors:     true,
		ParseComments: true,
	}
}

type goParseResult struct {
	Name    string
	AST     *ast.File
	FileSet *token.FileSet
	Errors  []error
}

func (f *goParseResult) HasErrors() bool {
	return f != nil && len(f.Errors) > 0
}

func parseGoSource(filename, src string, opts parseOptions) (*goParseResult, error) {
	fset := token.NewFileSet()
	return parseGoFileSet(fset, filename, src, opts)
}

func parseGoFileSet(fset *token.FileSet, filename, src string, opts parseOptions) (*goParseResult, error) {
	if fset == nil {
		return nil, fmt.Errorf("file set is required")
	}
	if filename == "" {
		filename = "input.go"
	}

	mode := opts.mode()
	astFile, err := goparser.ParseFile(fset, filename, src, mode)

	out := &goParseResult{
		Name:    filename,
		AST:     astFile,
		FileSet: fset,
	}
	if err != nil {
		out.Errors = []error{err}
		return out, err
	}
	return out, nil
}

func (o parseOptions) mode() goparser.Mode {
	var mode goparser.Mode
	if o.AllErrors {
		mode |= goparser.AllErrors
	}
	if o.ParseComments {
		mode |= goparser.ParseComments
	}
	if o.Trace {
		mode |= goparser.Trace
	}
	if o.DeclarationErrors {
		mode |= goparser.DeclarationErrors
	}
	return mode
}
