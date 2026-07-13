// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package typecheck

import (
	"go/types"
)

// Kind classifies types.Type nodes in the Go type hierarchy.
type Kind int

const (
	KindUnknown Kind = iota
	KindBasic
	KindNamed
	KindStruct
	KindInterface
	KindSlice
	KindMap
	KindPointer
	KindChan
	KindSignature
	KindArray
	KindTypeParam
	KindTuple
)

// Classify returns the primary kind of a Go type.
func Classify(t types.Type) Kind {
	if t == nil {
		return KindUnknown
	}
	switch t.(type) {
	case *types.Basic:
		return KindBasic
	case *types.Named:
		return KindNamed
	case *types.Struct:
		return KindStruct
	case *types.Interface:
		return KindInterface
	case *types.Slice:
		return KindSlice
	case *types.Map:
		return KindMap
	case *types.Pointer:
		return KindPointer
	case *types.Chan:
		return KindChan
	case *types.Signature:
		return KindSignature
	case *types.Array:
		return KindArray
	case *types.TypeParam:
		return KindTypeParam
	case *types.Tuple:
		return KindTuple
	default:
		return KindUnknown
	}
}

// IsInterface reports whether t is or resolves to an interface type.
func IsInterface(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// AsNamed unwraps a named type, following one pointer level.
func AsNamed(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

// AsStruct returns the struct type for t, including through named/pointer wrappers.
func AsStruct(t types.Type) *types.Struct {
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

// StructField describes one resolved struct field.
type StructField struct {
	Name string
	Type types.Type
	Tag  string
}

// StructFields lists fields for a struct type.
func StructFields(t types.Type) []StructField {
	strct := AsStruct(t)
	if strct == nil {
		return nil
	}
	fields := make([]StructField, 0, strct.NumFields())
	for i := 0; i < strct.NumFields(); i++ {
		f := strct.Field(i)
		fields = append(fields, StructField{
			Name: f.Name(),
			Type: f.Type(),
			Tag:  strct.Tag(i),
		})
	}
	return fields
}

// TypeArgCount returns the number of type arguments on a generic named type.
func TypeArgCount(t types.Type) int {
	named := AsNamed(t)
	if named == nil {
		return 0
	}
	args := named.TypeArgs()
	if args == nil {
		return 0
	}
	return args.Len()
}

// TypeArgAt returns the i-th type argument (e.g. Response[User] → User at 0).
func TypeArgAt(t types.Type, index int) types.Type {
	named := AsNamed(t)
	if named == nil {
		return nil
	}
	args := named.TypeArgs()
	if args == nil || index < 0 || index >= args.Len() {
		return nil
	}
	return args.At(index)
}

// TypeParamCount returns generic type parameter count on a named type.
func TypeParamCount(t types.Type) int {
	named := AsNamed(t)
	if named == nil {
		return 0
	}
	params := named.TypeParams()
	if params == nil {
		return 0
	}
	return params.Len()
}

// PackagePath returns the import path for a named type, if known.
func PackagePath(t types.Type) string {
	named := AsNamed(t)
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path()
}

// NamedTypeName returns the unqualified name of a named type.
func NamedTypeName(t types.Type) string {
	named := AsNamed(t)
	if named == nil || named.Obj() == nil {
		return ""
	}
	return named.Obj().Name()
}
