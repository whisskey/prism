// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package parser

import (
	"go/ast"
	"go/token"
)

// NodeKind returns a human-readable AST node type name.
func NodeKind(n ast.Node) string {
	if n == nil {
		return "nil"
	}
	switch n.(type) {
	case *ast.File:
		return "File"
	case *ast.FuncDecl:
		return "FuncDecl"
	case *ast.GenDecl:
		return "GenDecl"
	case *ast.ImportSpec:
		return "ImportSpec"
	case *ast.TypeSpec:
		return "TypeSpec"
	case *ast.ValueSpec:
		return "ValueSpec"
	case *ast.BadDecl:
		return "BadDecl"
	case *ast.AssignStmt:
		return "AssignStmt"
	case *ast.ReturnStmt:
		return "ReturnStmt"
	case *ast.ExprStmt:
		return "ExprStmt"
	case *ast.BlockStmt:
		return "BlockStmt"
	case *ast.IfStmt:
		return "IfStmt"
	case *ast.ForStmt:
		return "ForStmt"
	case *ast.RangeStmt:
		return "RangeStmt"
	case *ast.GoStmt:
		return "GoStmt"
	case *ast.CallExpr:
		return "CallExpr"
	case *ast.SelectorExpr:
		return "SelectorExpr"
	case *ast.Ident:
		return "Ident"
	case *ast.BasicLit:
		return "BasicLit"
	case *ast.CompositeLit:
		return "CompositeLit"
	case *ast.BinaryExpr:
		return "BinaryExpr"
	case *ast.UnaryExpr:
		return "UnaryExpr"
	case *ast.FuncLit:
		return "FuncLit"
	case *ast.IndexExpr:
		return "IndexExpr"
	case *ast.StructType:
		return "StructType"
	case *ast.Field:
		return "Field"
	case *ast.FieldList:
		return "FieldList"
	default:
		return "Node"
	}
}

// TypeSpecDecl finds a named type declaration in the file AST.
func TypeSpecDecl(file *ast.File, name string) *ast.TypeSpec {
	if file == nil {
		return nil
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if ok && typeSpec.Name != nil && typeSpec.Name.Name == name {
				return typeSpec
			}
		}
	}
	return nil
}

// StructFieldDecls returns struct field nodes for a named type, if it is a struct.
func StructFieldDecls(file *ast.File, typeName string) []*ast.Field {
	typeSpec := TypeSpecDecl(file, typeName)
	if typeSpec == nil {
		return nil
	}
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok || structType.Fields == nil {
		return nil
	}
	return structType.Fields.List
}
