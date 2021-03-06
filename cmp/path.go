// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package cmp

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

type (
	// Path is a list of PathSteps describing the sequence of operations to get
	// from some root type to the current position in the value tree.
	// The first Path element is always an operation-less PathStep that exists
	// simply to identify the initial type.
	//
	// When traversing structs with embedded structs, the embedded struct will
	// always be accessed as a field before traversing the fields of the
	// embedded struct themselves. That is, an exported field from the
	// embedded struct will never be accessed directly from the parent struct.
	Path []PathStep

	// PathStep is a union-type for specific operations to traverse
	// a value's tree structure. Users of this package never need to implement
	// these types as values of this type will be returned by this package.
	PathStep interface {
		String() string
		Type() reflect.Type // Resulting type after performing the path step
		isPathStep()
	}

	// SliceIndex is an index operation on a slice or array at some index Key.
	SliceIndex interface {
		PathStep
		Key() int
		isSliceIndex()
	}
	// MapIndex is an index operation on a map at some index Key.
	MapIndex interface {
		PathStep
		Key() reflect.Value
		isMapIndex()
	}
	// TypeAssertion represents a type assertion on an interface.
	TypeAssertion interface {
		PathStep
		isTypeAssertion()
	}
	// StructField represents a struct field access on a field called Name.
	StructField interface {
		PathStep
		Name() string
		Index() int
		isStructField()
	}
	// Indirect represents pointer indirection on the parent type.
	Indirect interface {
		PathStep
		isIndirect()
	}
	// Transform is a transformation from the parent type to the current type.
	Transform interface {
		PathStep
		Name() string
		Func() reflect.Value
		isTransform()
	}
)

func (pa *Path) push(s PathStep) {
	*pa = append(*pa, s)
}

func (pa *Path) pop() {
	*pa = (*pa)[:len(*pa)-1]
}

// String returns the simplified path to a node.
// The simplified path only contains struct field accesses.
//
// For example:
//	MyMap.MySlices.MyField
func (pa Path) String() string {
	var ss []string
	for _, s := range pa {
		if _, ok := s.(*structField); ok {
			ss = append(ss, s.String())
		}
	}
	return strings.TrimPrefix(strings.Join(ss, ""), ".")
}

// GoString returns the path to a specific node using Go syntax.
//
// For example:
//	(*root.MyMap["key"].(*mypkg.MyStruct).MySlices)[2][3].MyField
func (pa Path) GoString() string {
	var ssPre, ssPost []string
	var numIndirect int
	for i, s := range pa {
		var nextStep PathStep
		if i+1 < len(pa) {
			nextStep = pa[i+1]
		}
		switch s := s.(type) {
		case *indirect:
			numIndirect++
			pPre, pPost := "(", ")"
			switch nextStep.(type) {
			case *indirect:
				continue // Next step is indirection, so let them batch up
			case *structField:
				numIndirect-- // Automatic indirection on struct fields
			case nil:
				pPre, pPost = "", "" // Last step; no need for parenthesis
			}
			if numIndirect > 0 {
				ssPre = append(ssPre, pPre+strings.Repeat("*", numIndirect))
				ssPost = append(ssPost, pPost)
			}
			numIndirect = 0
			continue
		case *transform:
			ssPre = append(ssPre, s.trans.name+"(")
			ssPost = append(ssPost, ")")
			continue
		case *typeAssertion:
			// Elide type assertions immediately following a transform to
			// prevent overly verbose path printouts.
			// Some transforms return interface{} because of Go's lack of
			// generics, but typically take in and return the exact same
			// concrete type. Other times, the transform creates an anonymous
			// struct, which will be very verbose to print.
			if _, ok := nextStep.(*transform); ok {
				continue
			}
		}
		ssPost = append(ssPost, s.String())
	}
	for i, j := 0, len(ssPre)-1; i < j; i, j = i+1, j-1 {
		ssPre[i], ssPre[j] = ssPre[j], ssPre[i]
	}
	return strings.Join(ssPre, "") + strings.Join(ssPost, "")
}

type (
	pathStep struct {
		typ reflect.Type
	}

	sliceIndex struct {
		pathStep
		key int
	}
	mapIndex struct {
		pathStep
		key reflect.Value
	}
	typeAssertion struct {
		pathStep
	}
	structField struct {
		pathStep
		name string
		idx  int

		// These fields are used for forcibly accessing an unexported field.
		// pvx, pvy, and field are only valid if unexported is true.
		unexported bool
		force      bool                // Forcibly allow visibility
		pvx, pvy   reflect.Value       // Parent values
		field      reflect.StructField // Field information
	}
	indirect struct {
		pathStep
	}
	transform struct {
		pathStep
		trans *transformer
	}
)

func (ps pathStep) Type() reflect.Type { return ps.typ }
func (ps pathStep) String() string {
	s := ps.typ.String()
	if s == "" || strings.ContainsAny(s, "{}\n") {
		return "root" // Type too simple or complex to print
	}
	return "{" + s + "}"
}

func (si sliceIndex) String() string    { return fmt.Sprintf("[%d]", si.key) }
func (mi mapIndex) String() string      { return fmt.Sprintf("[%#v]", mi.key) }
func (ta typeAssertion) String() string { return fmt.Sprintf(".(%v)", ta.typ) }
func (sf structField) String() string   { return fmt.Sprintf(".%s", sf.name) }
func (in indirect) String() string      { return "*" }
func (tf transform) String() string     { return fmt.Sprintf("%s()", tf.trans.name) }

func (si sliceIndex) Key() int           { return si.key }
func (mi mapIndex) Key() reflect.Value   { return mi.key }
func (sf structField) Name() string      { return sf.name }
func (sf structField) Index() int        { return sf.idx }
func (tf transform) Name() string        { return tf.trans.name }
func (tf transform) Func() reflect.Value { return tf.trans.fnc }

func (pathStep) isPathStep()           {}
func (sliceIndex) isSliceIndex()       {}
func (mapIndex) isMapIndex()           {}
func (typeAssertion) isTypeAssertion() {}
func (structField) isStructField()     {}
func (indirect) isIndirect()           {}
func (transform) isTransform()         {}

var (
	_ SliceIndex    = sliceIndex{}
	_ MapIndex      = mapIndex{}
	_ TypeAssertion = typeAssertion{}
	_ StructField   = structField{}
	_ Indirect      = indirect{}
	_ Transform     = transform{}

	_ PathStep = sliceIndex{}
	_ PathStep = mapIndex{}
	_ PathStep = typeAssertion{}
	_ PathStep = structField{}
	_ PathStep = indirect{}
	_ PathStep = transform{}
)

// isExported reports whether the identifier is exported.
func isExported(id string) bool {
	r, _ := utf8.DecodeRuneInString(id)
	return unicode.IsUpper(r)
}

// isValid reports whether the identifier is valid.
// Empty and underscore-only strings are not valid.
func isValid(id string) bool {
	ok := id != "" && id != "_"
	for j, c := range id {
		ok = ok && (j > 0 || !unicode.IsDigit(c))
		ok = ok && (c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c))
	}
	return ok
}
