// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/types"
	"strings"
)

// TypeName returns a stable Go type string suitable for schema lookup.
func TypeName(t types.Type) string {
	if t == nil {
		return ""
	}
	return types.TypeString(t, func(*types.Package) string { return "" })
}

// FirstResultType extracts the first result type from multi-value types.
func FirstResultType(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if tuple, ok := t.(*types.Tuple); ok && tuple.Len() > 0 {
		return tuple.At(0).Type()
	}
	return t
}

// SchemaName extracts the named struct type from t, stripping pointers and package paths.
func SchemaName(t types.Type) string {
	t = FirstResultType(t)
	for {
		switch v := t.(type) {
		case *types.Pointer:
			t = v.Elem()
			continue
		case *types.Slice, *types.Array:
			elem := SchemaName(elementType(v))
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
			name := TypeName(t)
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

// MethodReturnType returns the first result type of a method on a named receiver type.
func MethodReturnType(recvType types.Type, method string) types.Type {
	if recvType == nil || method == "" {
		return nil
	}
	obj := lookupMethod(recvType, method)
	if obj == nil {
		return nil
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok || sig.Results().Len() == 0 {
		return nil
	}
	return sig.Results().At(0).Type()
}

func lookupMethod(recvType types.Type, method string) *types.Func {
	switch t := recvType.(type) {
	case *types.Named:
		if m, _, _ := types.LookupFieldOrMethod(t, true, t.Obj().Pkg(), method); m != nil {
			if fn, ok := m.(*types.Func); ok {
				return fn
			}
		}
	case *types.Pointer:
		elem := t.Elem()
		if named, ok := elem.(*types.Named); ok {
			if m, _, _ := types.LookupFieldOrMethod(t, false, named.Obj().Pkg(), method); m != nil {
				if fn, ok := m.(*types.Func); ok {
					return fn
				}
			}
		}
	}
	return nil
}

// FuncReturnType returns the first result type of a package-level function.
func FuncReturnType(pkg *types.Package, name string) types.Type {
	if pkg == nil || name == "" {
		return nil
	}
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Results().Len() == 0 {
		return nil
	}
	return sig.Results().At(0).Type()
}
