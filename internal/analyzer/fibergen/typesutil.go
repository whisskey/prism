// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func firstResultType(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if tuple, ok := t.(*types.Tuple); ok && tuple.Len() > 0 {
		return tuple.At(0).Type()
	}
	return t
}

func schemaName(t types.Type) string {
	t = firstResultType(t)
	for {
		switch v := t.(type) {
		case *types.Pointer:
			t = v.Elem()
			continue
		case *types.Slice, *types.Array:
			elem := schemaName(elementType(v))
			if elem == "" {
				return ""
			}
			return "[]" + elem
		case *types.Named:
			if isPrimitiveNamed(v) {
				return ""
			}
			return v.Obj().Name()
		case *types.Basic:
			return ""
		case *types.Tuple:
			if v.Len() > 0 {
				t = v.At(0).Type()
				continue
			}
			return ""
		default:
			name := types.TypeString(t, func(*types.Package) string { return "" })
			if idx := strings.LastIndex(name, "."); idx >= 0 {
				name = name[idx+1:]
			}
			name = strings.TrimPrefix(name, "*")
			if isPrimitiveName(name) {
				return ""
			}
			return name
		}
	}
}

func elementType(t types.Type) types.Type {
	switch v := t.(type) {
	case *types.Slice:
		return v.Elem()
	case *types.Array:
		return v.Elem()
	default:
		return nil
	}
}

func isPrimitiveNamed(n *types.Named) bool {
	if n == nil || n.Obj() == nil || n.Obj().Pkg() == nil {
		return false
	}
	return n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Time"
}

func isPrimitiveName(name string) bool {
	switch name {
	case "", "any", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64",
		"bool", "byte", "rune", "error", "interface{}":
		return true
	default:
		return false
	}
}

func asNamed(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

func typeBaseName(typ types.Type) string {
	if typ == nil {
		return ""
	}
	if named, ok := typ.(*types.Named); ok {
		if args := named.TypeArgs(); args != nil && args.Len() > 0 {
			if arg := typeBaseName(args.At(0)); arg != "" {
				return arg
			}
		}
		if isPrimitiveNamed(named) {
			return ""
		}
		return named.Obj().Name()
	}
	s := typ.String()
	if isPrimitiveName(s) {
		return ""
	}
	return s
}

func typeNameFromGoType(typ types.Type) string {
	if typ == nil {
		return ""
	}
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	if slice, ok := typ.(*types.Slice); ok {
		return "[]" + typeBaseName(slice.Elem())
	}
	if arr, ok := typ.(*types.Array); ok {
		return "[]" + typeBaseName(arr.Elem())
	}
	return typeBaseName(typ)
}

func resolveTypeName(pass *analysis.Pass, expr ast.Expr, locals map[string]string) string {
	if expr == nil {
		return ""
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if name, ok := locals[ident.Name]; ok {
			return name
		}
	}
	if pass != nil && pass.TypesInfo != nil {
		if typ := pass.TypesInfo.TypeOf(expr); typ != nil {
			if name := typeNameFromGoType(typ); name != "" {
				return name
			}
		}
	}
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			return resolveTypeName(pass, e.X, locals)
		}
	case *ast.CompositeLit:
		if name := genericTypeFromASTExpr(e.Type); name != "" {
			return name
		}
		return typeNameFromASTExpr(e.Type)
	case *ast.Ident:
		return locals[e.Name]
	}
	return ""
}

func genericTypeFromASTExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IndexExpr:
		return typeNameFromASTExpr(e.Index)
	case *ast.IndexListExpr:
		if len(e.Indices) > 0 {
			return typeNameFromASTExpr(e.Indices[0])
		}
	}
	return ""
}

func typeNameFromASTExpr(expr ast.Expr) string {
	if name := genericTypeFromASTExpr(expr); name != "" {
		return name
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.StarExpr:
		return typeNameFromASTExpr(e.X)
	default:
		return ""
	}
}

func asStruct(t types.Type) *types.Struct {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		if strct, ok := named.Underlying().(*types.Struct); ok {
			return strct
		}
	}
	strct, _ := t.(*types.Struct)
	return strct
}

func isAnyType(t types.Type) bool {
	if t == nil {
		return false
	}
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}
