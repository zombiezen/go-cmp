// Copyright 2017, The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package cmp_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	pb "github.com/google/go-cmp/cmp/internal/testprotos"
	ts "github.com/google/go-cmp/cmp/internal/teststructs"
)

var now = time.Now()
var boolType = reflect.TypeOf(true)
var mutexType = reflect.TypeOf(sync.Mutex{})

func intPtr(n int) *int { return &n }

func equalRegexp(x, y *regexp.Regexp) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}
	return x.String() == y.String()
}

func IgnoreUnexported(typs ...interface{}) cmp.Option {
	m := make(map[reflect.Type]bool)
	for _, typ := range typs {
		t := reflect.TypeOf(typ)
		if t.Kind() != reflect.Struct {
			panic(fmt.Sprintf("invalid struct type: %T", typ))
		}
		m[t] = true
	}
	return cmp.FilterPath(func(p cmp.Path) bool {
		if len(p) < 2 {
			return false
		}
		sf, ok := p[len(p)-1].(cmp.StructField)
		if !ok {
			return false
		}
		return m[p[len(p)-2].Type()] && !isExported(sf.Name())
	}, cmp.Ignore())
}

func isExported(id string) bool {
	r, _ := utf8.DecodeRuneInString(id)
	return unicode.IsUpper(r)
}

type test struct {
	label     string       // Test description
	x, y      interface{}  // Input values to compare
	opts      []cmp.Option // Input options
	wantDiff  string       // The exact difference string
	wantPanic string       // Sub-string of an expected panic message
}

func TestDiff(t *testing.T) {
	var tests []test
	tests = append(tests, comparerTests()...)
	tests = append(tests, transformerTests()...)
	tests = append(tests, embeddedTests()...)
	tests = append(tests, methodTests()...)
	tests = append(tests, project1Tests()...)
	tests = append(tests, project2Tests()...)
	tests = append(tests, project3Tests()...)
	tests = append(tests, project4Tests()...)

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			var gotDiff, gotPanic string
			func() {
				defer func() {
					if ex := recover(); ex != nil {
						if s, ok := ex.(string); ok {
							gotPanic = s
						} else {
							panic(ex)
						}
					}
				}()
				gotDiff = cmp.Diff(tt.x, tt.y, tt.opts...)
			}()
			if tt.wantPanic == "" {
				if gotPanic != "" {
					t.Fatalf("unexpected panic message: %s", gotPanic)
				}
				if strings.TrimSpace(gotDiff) != strings.TrimSpace(tt.wantDiff) {
					t.Fatalf("difference message:\ngot:\n%s\nwant:\n%s", gotDiff, tt.wantDiff)
				}
			} else {
				if !strings.Contains(gotPanic, tt.wantPanic) {
					t.Fatalf("panic message:\ngot:  %s\nwant: %s", gotPanic, tt.wantPanic)
				}
			}
		})
	}
}

func comparerTests() []test {
	const label = "Comparer"

	return []test{{
		label:    label,
		x:        1,
		y:        1,
		wantDiff: "",
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Ignore()},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Comparer(func(_, _ interface{}) bool { return true })},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label:     label,
		x:         1,
		y:         1,
		opts:      []cmp.Option{cmp.Transformer("", func(x interface{}) interface{} { return x })},
		wantPanic: "cannot use an unfiltered option",
	}, {
		label: label,
		x:     1,
		y:     1,
		opts: []cmp.Option{
			cmp.Comparer(func(x, y int) bool { return true }),
			cmp.Transformer("", func(x int) float64 { return float64(x) }),
		},
		wantPanic: "ambiguous set of options",
	}, {
		label: label,
		x:     1,
		y:     1,
		opts: []cmp.Option{
			cmp.FilterPath(func(p cmp.Path) bool {
				return len(p) > 0 && p[len(p)-1].Type().Kind() == reflect.Int
			}, cmp.Options{cmp.Ignore(), cmp.Ignore(), cmp.Ignore()}),
			cmp.Comparer(func(x, y int) bool { return true }),
			cmp.Transformer("", func(x int) float64 { return float64(x) }),
		},
	}, {
		label:     label,
		opts:      []cmp.Option{struct{ cmp.Option }{}},
		wantPanic: "unknown option",
	}, {
		label:    label,
		x:        struct{ A, B, C int }{1, 2, 3},
		y:        struct{ A, B, C int }{1, 2, 3},
		wantDiff: "",
	}, {
		label:    label,
		x:        struct{ A, B, C int }{1, 2, 3},
		y:        struct{ A, B, C int }{1, 2, 4},
		wantDiff: "root.C:\n\t-: 3\n\t+: 4\n",
	}, {
		label:     label,
		x:         struct{ a, b, c int }{1, 2, 3},
		y:         struct{ a, b, c int }{1, 2, 4},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(4)},
	}, {
		label:    label,
		x:        &struct{ A *int }{intPtr(4)},
		y:        &struct{ A *int }{intPtr(5)},
		wantDiff: "*root.A:\n\t-: 4\n\t+: 5\n",
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(5)},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y int) bool { return true }),
		},
	}, {
		label: label,
		x:     &struct{ A *int }{intPtr(4)},
		y:     &struct{ A *int }{intPtr(5)},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y *int) bool { return x != nil && y != nil }),
		},
	}, {
		label: label,
		x:     &struct{ R *bytes.Buffer }{},
		y:     &struct{ R *bytes.Buffer }{},
	}, {
		label:    label,
		x:        &struct{ R *bytes.Buffer }{new(bytes.Buffer)},
		y:        &struct{ R *bytes.Buffer }{},
		wantDiff: "root.R:\n\t-: \"\"\n\t+: <nil>\n",
	}, {
		label: label,
		x:     &struct{ R *bytes.Buffer }{new(bytes.Buffer)},
		y:     &struct{ R *bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
	}, {
		label:     label,
		x:         &struct{ R bytes.Buffer }{},
		y:         &struct{ R bytes.Buffer }{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ R bytes.Buffer }{},
		y:     &struct{ R bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     &struct{ R bytes.Buffer }{},
		y:     &struct{ R bytes.Buffer }{},
		opts: []cmp.Option{
			cmp.Transformer("Ref", func(x bytes.Buffer) *bytes.Buffer { return &x }),
			cmp.Comparer(func(x, y io.Reader) bool { return true }),
		},
	}, {
		label:     label,
		x:         []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:         []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		wantPanic: "cannot handle unexported field",
	}, {
		label:    label,
		x:        []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:        []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		opts:     []cmp.Option{cmp.Comparer(equalRegexp)},
		wantDiff: "",
	}, {
		label:    label,
		x:        []*regexp.Regexp{nil, regexp.MustCompile("a*b*c*")},
		y:        []*regexp.Regexp{nil, regexp.MustCompile("a*b*d*")},
		opts:     []cmp.Option{cmp.Comparer(equalRegexp)},
		wantDiff: "{[]*regexp.Regexp}[1]:\n\t-: \"a*b*c*\"\n\t+: \"a*b*d*\"\n",
	}, {
		label: label,
		x: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
		y: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
	}, {
		label: label,
		x: func() ***int {
			a := 0
			b := &a
			c := &b
			return &c
		}(),
		y: func() ***int {
			a := 1
			b := &a
			c := &b
			return &c
		}(),
		wantDiff: `
***{***int}:
	-: 0
	+: 1`,
	}, {
		label: label,
		x:     []int{1, 2, 3, 4, 5}[:3],
		y:     []int{1, 2, 3},
	}, {
		label: label,
		x:     struct{ fmt.Stringer }{bytes.NewBufferString("hello")},
		y:     struct{ fmt.Stringer }{regexp.MustCompile("hello")},
		opts:  []cmp.Option{cmp.Comparer(func(x, y fmt.Stringer) bool { return x.String() == y.String() })},
	}, {
		label: label,
		x:     struct{ fmt.Stringer }{bytes.NewBufferString("hello")},
		y:     struct{ fmt.Stringer }{regexp.MustCompile("hello2")},
		opts:  []cmp.Option{cmp.Comparer(func(x, y fmt.Stringer) bool { return x.String() == y.String() })},
		wantDiff: `
root:
	-: "hello"
	+: "hello2"`,
	}, {
		label: label,
		x:     make([]int, 1000),
		y:     make([]int, 1000),
		opts: []cmp.Option{
			cmp.Comparer(func(_, _ int) bool {
				return rand.Intn(2) == 0
			}),
		},
		wantPanic: "non-deterministic or non-symmetric function detected",
	}, {
		label: label,
		x:     make([]int, 1000),
		y:     make([]int, 1000),
		opts: []cmp.Option{
			cmp.FilterValues(func(_, _ int) bool {
				return rand.Intn(2) == 0
			}, cmp.Ignore()),
		},
		wantPanic: "non-deterministic or non-symmetric function detected",
	}}
}

func transformerTests() []test {
	const label = "Transformer/"

	return []test{{
		label: label,
		x:     uint8(0),
		y:     uint8(1),
		opts: []cmp.Option{
			cmp.Transformer("", func(in uint8) uint16 { return uint16(in) }),
			cmp.Transformer("", func(in uint16) uint32 { return uint32(in) }),
			cmp.Transformer("", func(in uint32) uint64 { return uint64(in) }),
		},
		wantDiff: `
λ(λ(λ({uint8}))):
	-: 0x00
	+: 0x01`,
	}, {
		label: label,
		x:     0,
		y:     1,
		opts: []cmp.Option{
			cmp.Transformer("", func(in int) int { return in / 2 }),
			cmp.Transformer("", func(in int) int { return in }),
		},
		wantPanic: "ambiguous set of options",
	}, {
		label: label,
		x:     []int{0, -5, 0, -1},
		y:     []int{1, 3, 0, -5},
		opts: []cmp.Option{
			cmp.FilterValues(
				func(x, y int) bool { return x+y >= 0 },
				cmp.Transformer("", func(in int) int64 { return int64(in / 2) }),
			),
			cmp.FilterValues(
				func(x, y int) bool { return x+y < 0 },
				cmp.Transformer("", func(in int) int64 { return int64(in) }),
			),
		},
		wantDiff: `
λ({[]int}[1]):
	-: -5
	+: 3
λ({[]int}[3]):
	-: -1
	+: -5`,
	}, {
		label: label,
		x:     0,
		y:     1,
		opts: []cmp.Option{
			cmp.Transformer("", func(in int) interface{} {
				if in == 0 {
					return "string"
				}
				return in
			}),
		},
		wantDiff: `
λ({int}):
	-: "string"
	+: 1`,
	}}
}

func embeddedTests() []test {
	const label = "EmbeddedStruct/"

	privateStruct := *new(ts.ParentStructA).PrivateStruct()

	createStructA := func(i int) ts.ParentStructA {
		s := ts.ParentStructA{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		return s
	}

	createStructB := func(i int) ts.ParentStructB {
		s := ts.ParentStructB{}
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		return s
	}

	createStructC := func(i int) ts.ParentStructC {
		s := ts.ParentStructC{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.Public = 3 + i
		s.SetPrivate(4 + i)
		return s
	}

	createStructD := func(i int) ts.ParentStructD {
		s := ts.ParentStructD{}
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		s.Public = 3 + i
		s.SetPrivate(4 + i)
		return s
	}

	createStructE := func(i int) ts.ParentStructE {
		s := ts.ParentStructE{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		return s
	}

	createStructF := func(i int) ts.ParentStructF {
		s := ts.ParentStructF{}
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		s.Public = 5 + i
		s.SetPrivate(6 + i)
		return s
	}

	createStructG := func(i int) *ts.ParentStructG {
		s := ts.NewParentStructG()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		return s
	}

	createStructH := func(i int) *ts.ParentStructH {
		s := ts.NewParentStructH()
		s.PublicStruct.Public = 1 + i
		s.PublicStruct.SetPrivate(2 + i)
		return s
	}

	createStructI := func(i int) *ts.ParentStructI {
		s := ts.NewParentStructI()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		return s
	}

	createStructJ := func(i int) *ts.ParentStructJ {
		s := ts.NewParentStructJ()
		s.PrivateStruct().Public = 1 + i
		s.PrivateStruct().SetPrivate(2 + i)
		s.PublicStruct.Public = 3 + i
		s.PublicStruct.SetPrivate(4 + i)
		s.Private().Public = 5 + i
		s.Private().SetPrivate(6 + i)
		s.Public.Public = 7 + i
		s.Public.SetPrivate(8 + i)
		return s
	}

	return []test{{
		label:     label + "ParentStructA",
		x:         ts.ParentStructA{},
		y:         ts.ParentStructA{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructA",
		x:     ts.ParentStructA{},
		y:     ts.ParentStructA{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructA{}),
		},
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}, privateStruct),
		},
	}, {
		label: label + "ParentStructA",
		x:     createStructA(0),
		y:     createStructA(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructA{}, privateStruct),
		},
		wantDiff: `
{teststructs.ParentStructA}.privateStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructA}.privateStruct.private:
	-: 2
	+: 3`,
	}, {
		label: label + "ParentStructB",
		x:     ts.ParentStructB{},
		y:     ts.ParentStructB{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructB{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructB",
		x:     ts.ParentStructB{},
		y:     ts.ParentStructB{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructB{}),
			IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructB",
		x:     createStructB(0),
		y:     createStructB(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructB{}, ts.PublicStruct{}),
		},
		wantDiff: `
{teststructs.ParentStructB}.PublicStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructB}.PublicStruct.private:
	-: 2
	+: 3`,
	}, {
		label:     label + "ParentStructC",
		x:         ts.ParentStructC{},
		y:         ts.ParentStructC{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructC",
		x:     ts.ParentStructC{},
		y:     ts.ParentStructC{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructC{}),
		},
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}, privateStruct),
		},
	}, {
		label: label + "ParentStructC",
		x:     createStructC(0),
		y:     createStructC(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructC{}, privateStruct),
		},
		wantDiff: `
{teststructs.ParentStructC}.privateStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructC}.privateStruct.private:
	-: 2
	+: 3
{teststructs.ParentStructC}.Public:
	-: 3
	+: 4
{teststructs.ParentStructC}.private:
	-: 4
	+: 5`,
	}, {
		label: label + "ParentStructD",
		x:     ts.ParentStructD{},
		y:     ts.ParentStructD{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructD{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructD",
		x:     ts.ParentStructD{},
		y:     ts.ParentStructD{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructD{}),
			IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructD",
		x:     createStructD(0),
		y:     createStructD(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructD{}, ts.PublicStruct{}),
		},
		wantDiff: `
{teststructs.ParentStructD}.PublicStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructD}.PublicStruct.private:
	-: 2
	+: 3
{teststructs.ParentStructD}.Public:
	-: 3
	+: 4
{teststructs.ParentStructD}.private:
	-: 4
	+: 5`,
	}, {
		label: label + "ParentStructE",
		x:     ts.ParentStructE{},
		y:     ts.ParentStructE{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructE{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     ts.ParentStructE{},
		y:     ts.ParentStructE{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructE{}),
			IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructE",
		x:     createStructE(0),
		y:     createStructE(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructE{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
{teststructs.ParentStructE}.privateStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructE}.privateStruct.private:
	-: 2
	+: 3
{teststructs.ParentStructE}.PublicStruct.Public:
	-: 3
	+: 4
{teststructs.ParentStructE}.PublicStruct.private:
	-: 4
	+: 5`,
	}, {
		label: label + "ParentStructF",
		x:     ts.ParentStructF{},
		y:     ts.ParentStructF{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructF{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     ts.ParentStructF{},
		y:     ts.ParentStructF{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructF{}),
			IgnoreUnexported(ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructF",
		x:     createStructF(0),
		y:     createStructF(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructF{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
{teststructs.ParentStructF}.privateStruct.Public:
	-: 1
	+: 2
{teststructs.ParentStructF}.privateStruct.private:
	-: 2
	+: 3
{teststructs.ParentStructF}.PublicStruct.Public:
	-: 3
	+: 4
{teststructs.ParentStructF}.PublicStruct.private:
	-: 4
	+: 5
{teststructs.ParentStructF}.Public:
	-: 5
	+: 6
{teststructs.ParentStructF}.private:
	-: 6
	+: 7`,
	}, {
		label:     label + "ParentStructG",
		x:         ts.ParentStructG{},
		y:         ts.ParentStructG{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructG",
		x:     ts.ParentStructG{},
		y:     ts.ParentStructG{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructG{}),
		},
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}, privateStruct),
		},
	}, {
		label: label + "ParentStructG",
		x:     createStructG(0),
		y:     createStructG(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructG{}, privateStruct),
		},
		wantDiff: `
{*teststructs.ParentStructG}.privateStruct.Public:
	-: 1
	+: 2
{*teststructs.ParentStructG}.privateStruct.private:
	-: 2
	+: 3`,
	}, {
		label: label + "ParentStructH",
		x:     ts.ParentStructH{},
		y:     ts.ParentStructH{},
	}, {
		label:     label + "ParentStructH",
		x:         createStructH(0),
		y:         createStructH(0),
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructH",
		x:     ts.ParentStructH{},
		y:     ts.ParentStructH{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructH{}),
		},
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructH",
		x:     createStructH(0),
		y:     createStructH(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructH{}, ts.PublicStruct{}),
		},
		wantDiff: `
{*teststructs.ParentStructH}.PublicStruct.Public:
	-: 1
	+: 2
{*teststructs.ParentStructH}.PublicStruct.private:
	-: 2
	+: 3`,
	}, {
		label:     label + "ParentStructI",
		x:         ts.ParentStructI{},
		y:         ts.ParentStructI{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructI",
		x:     ts.ParentStructI{},
		y:     ts.ParentStructI{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructI{}),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructI{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructI{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructI",
		x:     createStructI(0),
		y:     createStructI(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructI{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
{*teststructs.ParentStructI}.privateStruct.Public:
	-: 1
	+: 2
{*teststructs.ParentStructI}.privateStruct.private:
	-: 2
	+: 3
{*teststructs.ParentStructI}.PublicStruct.Public:
	-: 3
	+: 4
{*teststructs.ParentStructI}.PublicStruct.private:
	-: 4
	+: 5`,
	}, {
		label:     label + "ParentStructJ",
		x:         ts.ParentStructJ{},
		y:         ts.ParentStructJ{},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     ts.ParentStructJ{},
		y:     ts.ParentStructJ{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructJ{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     ts.ParentStructJ{},
		y:     ts.ParentStructJ{},
		opts: []cmp.Option{
			IgnoreUnexported(ts.ParentStructJ{}, ts.PublicStruct{}),
		},
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}),
		},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(0),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}, privateStruct),
		},
	}, {
		label: label + "ParentStructJ",
		x:     createStructJ(0),
		y:     createStructJ(1),
		opts: []cmp.Option{
			cmp.AllowUnexported(ts.ParentStructJ{}, ts.PublicStruct{}, privateStruct),
		},
		wantDiff: `
{*teststructs.ParentStructJ}.privateStruct.Public:
	-: 1
	+: 2
{*teststructs.ParentStructJ}.privateStruct.private:
	-: 2
	+: 3
{*teststructs.ParentStructJ}.PublicStruct.Public:
	-: 3
	+: 4
{*teststructs.ParentStructJ}.PublicStruct.private:
	-: 4
	+: 5
{*teststructs.ParentStructJ}.Public.Public:
	-: 7
	+: 8
{*teststructs.ParentStructJ}.Public.private:
	-: 8
	+: 9
{*teststructs.ParentStructJ}.private.Public:
	-: 5
	+: 6
{*teststructs.ParentStructJ}.private.private:
	-: 6
	+: 7`,
	}}
}

func methodTests() []test {
	const label = "EqualMethod/"

	// A common mistake that the Equal method is on a pointer receiver,
	// but only a non-pointer value is present in the struct.
	// A transform can be used to forcibly reference the value.
	derefTransform := cmp.FilterPath(func(p cmp.Path) bool {
		if len(p) == 0 {
			return false
		}
		t := p[len(p)-1].Type()
		if _, ok := t.MethodByName("Equal"); ok || t.Kind() == reflect.Ptr {
			return false
		}
		if m, ok := reflect.PtrTo(t).MethodByName("Equal"); ok {
			tf := m.Func.Type()
			return !tf.IsVariadic() && tf.NumIn() == 2 && tf.NumOut() == 1 &&
				tf.In(0).AssignableTo(tf.In(1)) && tf.Out(0) == boolType
		}
		return false
	}, cmp.Transformer("Ref", func(x interface{}) interface{} {
		v := reflect.ValueOf(x)
		vp := reflect.New(v.Type())
		vp.Elem().Set(v)
		return vp.Interface()
	}))

	// For each of these types, there is an Equal method defined, which always
	// returns true, while the underlying data are fundamentally different.
	// Since the method should be called, these are expected to be equal.
	return []test{{
		label: label + "StructA",
		x:     ts.StructA{"NotEqual"},
		y:     ts.StructA{"not_equal"},
	}, {
		label: label + "StructA",
		x:     &ts.StructA{"NotEqual"},
		y:     &ts.StructA{"not_equal"},
	}, {
		label: label + "StructB",
		x:     ts.StructB{"NotEqual"},
		y:     ts.StructB{"not_equal"},
		wantDiff: `
{teststructs.StructB}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructB",
		x:     ts.StructB{"NotEqual"},
		y:     ts.StructB{"not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructB",
		x:     &ts.StructB{"NotEqual"},
		y:     &ts.StructB{"not_equal"},
	}, {
		label: label + "StructC",
		x:     ts.StructC{"NotEqual"},
		y:     ts.StructC{"not_equal"},
	}, {
		label: label + "StructC",
		x:     &ts.StructC{"NotEqual"},
		y:     &ts.StructC{"not_equal"},
	}, {
		label: label + "StructD",
		x:     ts.StructD{"NotEqual"},
		y:     ts.StructD{"not_equal"},
		wantDiff: `
{teststructs.StructD}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructD",
		x:     ts.StructD{"NotEqual"},
		y:     ts.StructD{"not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructD",
		x:     &ts.StructD{"NotEqual"},
		y:     &ts.StructD{"not_equal"},
	}, {
		label: label + "StructE",
		x:     ts.StructE{"NotEqual"},
		y:     ts.StructE{"not_equal"},
		wantDiff: `
{teststructs.StructE}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructE",
		x:     ts.StructE{"NotEqual"},
		y:     ts.StructE{"not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructE",
		x:     &ts.StructE{"NotEqual"},
		y:     &ts.StructE{"not_equal"},
	}, {
		label: label + "StructF",
		x:     ts.StructF{"NotEqual"},
		y:     ts.StructF{"not_equal"},
		wantDiff: `
{teststructs.StructF}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructF",
		x:     &ts.StructF{"NotEqual"},
		y:     &ts.StructF{"not_equal"},
	}, {
		label: label + "StructA1",
		x:     ts.StructA1{ts.StructA{"NotEqual"}, "equal"},
		y:     ts.StructA1{ts.StructA{"not_equal"}, "equal"},
	}, {
		label:    label + "StructA1",
		x:        ts.StructA1{ts.StructA{"NotEqual"}, "NotEqual"},
		y:        ts.StructA1{ts.StructA{"not_equal"}, "not_equal"},
		wantDiff: "{teststructs.StructA1}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructA1",
		x:     &ts.StructA1{ts.StructA{"NotEqual"}, "equal"},
		y:     &ts.StructA1{ts.StructA{"not_equal"}, "equal"},
	}, {
		label:    label + "StructA1",
		x:        &ts.StructA1{ts.StructA{"NotEqual"}, "NotEqual"},
		y:        &ts.StructA1{ts.StructA{"not_equal"}, "not_equal"},
		wantDiff: "{*teststructs.StructA1}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructB1",
		x:     ts.StructB1{ts.StructB{"NotEqual"}, "equal"},
		y:     ts.StructB1{ts.StructB{"not_equal"}, "equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label:    label + "StructB1",
		x:        ts.StructB1{ts.StructB{"NotEqual"}, "NotEqual"},
		y:        ts.StructB1{ts.StructB{"not_equal"}, "not_equal"},
		opts:     []cmp.Option{derefTransform},
		wantDiff: "{teststructs.StructB1}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructB1",
		x:     &ts.StructB1{ts.StructB{"NotEqual"}, "equal"},
		y:     &ts.StructB1{ts.StructB{"not_equal"}, "equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label:    label + "StructB1",
		x:        &ts.StructB1{ts.StructB{"NotEqual"}, "NotEqual"},
		y:        &ts.StructB1{ts.StructB{"not_equal"}, "not_equal"},
		opts:     []cmp.Option{derefTransform},
		wantDiff: "{*teststructs.StructB1}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructC1",
		x:     ts.StructC1{ts.StructC{"NotEqual"}, "NotEqual"},
		y:     ts.StructC1{ts.StructC{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructC1",
		x:     &ts.StructC1{ts.StructC{"NotEqual"}, "NotEqual"},
		y:     &ts.StructC1{ts.StructC{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructD1",
		x:     ts.StructD1{ts.StructD{"NotEqual"}, "NotEqual"},
		y:     ts.StructD1{ts.StructD{"not_equal"}, "not_equal"},
		wantDiff: `
{teststructs.StructD1}.StructD.X:
	-: "NotEqual"
	+: "not_equal"
{teststructs.StructD1}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructD1",
		x:     ts.StructD1{ts.StructD{"NotEqual"}, "NotEqual"},
		y:     ts.StructD1{ts.StructD{"not_equal"}, "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructD1",
		x:     &ts.StructD1{ts.StructD{"NotEqual"}, "NotEqual"},
		y:     &ts.StructD1{ts.StructD{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructE1",
		x:     ts.StructE1{ts.StructE{"NotEqual"}, "NotEqual"},
		y:     ts.StructE1{ts.StructE{"not_equal"}, "not_equal"},
		wantDiff: `
{teststructs.StructE1}.StructE.X:
	-: "NotEqual"
	+: "not_equal"
{teststructs.StructE1}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructE1",
		x:     ts.StructE1{ts.StructE{"NotEqual"}, "NotEqual"},
		y:     ts.StructE1{ts.StructE{"not_equal"}, "not_equal"},
		opts:  []cmp.Option{derefTransform},
	}, {
		label: label + "StructE1",
		x:     &ts.StructE1{ts.StructE{"NotEqual"}, "NotEqual"},
		y:     &ts.StructE1{ts.StructE{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructF1",
		x:     ts.StructF1{ts.StructF{"NotEqual"}, "NotEqual"},
		y:     ts.StructF1{ts.StructF{"not_equal"}, "not_equal"},
		wantDiff: `
{teststructs.StructF1}.StructF.X:
	-: "NotEqual"
	+: "not_equal"
{teststructs.StructF1}.X:
	-: "NotEqual"
	+: "not_equal"`,
	}, {
		label: label + "StructF1",
		x:     &ts.StructF1{ts.StructF{"NotEqual"}, "NotEqual"},
		y:     &ts.StructF1{ts.StructF{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructA2",
		x:     ts.StructA2{&ts.StructA{"NotEqual"}, "equal"},
		y:     ts.StructA2{&ts.StructA{"not_equal"}, "equal"},
	}, {
		label:    label + "StructA2",
		x:        ts.StructA2{&ts.StructA{"NotEqual"}, "NotEqual"},
		y:        ts.StructA2{&ts.StructA{"not_equal"}, "not_equal"},
		wantDiff: "{teststructs.StructA2}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructA2",
		x:     &ts.StructA2{&ts.StructA{"NotEqual"}, "equal"},
		y:     &ts.StructA2{&ts.StructA{"not_equal"}, "equal"},
	}, {
		label:    label + "StructA2",
		x:        &ts.StructA2{&ts.StructA{"NotEqual"}, "NotEqual"},
		y:        &ts.StructA2{&ts.StructA{"not_equal"}, "not_equal"},
		wantDiff: "{*teststructs.StructA2}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructB2",
		x:     ts.StructB2{&ts.StructB{"NotEqual"}, "equal"},
		y:     ts.StructB2{&ts.StructB{"not_equal"}, "equal"},
	}, {
		label:    label + "StructB2",
		x:        ts.StructB2{&ts.StructB{"NotEqual"}, "NotEqual"},
		y:        ts.StructB2{&ts.StructB{"not_equal"}, "not_equal"},
		wantDiff: "{teststructs.StructB2}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructB2",
		x:     &ts.StructB2{&ts.StructB{"NotEqual"}, "equal"},
		y:     &ts.StructB2{&ts.StructB{"not_equal"}, "equal"},
	}, {
		label:    label + "StructB2",
		x:        &ts.StructB2{&ts.StructB{"NotEqual"}, "NotEqual"},
		y:        &ts.StructB2{&ts.StructB{"not_equal"}, "not_equal"},
		wantDiff: "{*teststructs.StructB2}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "StructC2",
		x:     ts.StructC2{&ts.StructC{"NotEqual"}, "NotEqual"},
		y:     ts.StructC2{&ts.StructC{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructC2",
		x:     &ts.StructC2{&ts.StructC{"NotEqual"}, "NotEqual"},
		y:     &ts.StructC2{&ts.StructC{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructD2",
		x:     ts.StructD2{&ts.StructD{"NotEqual"}, "NotEqual"},
		y:     ts.StructD2{&ts.StructD{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructD2",
		x:     &ts.StructD2{&ts.StructD{"NotEqual"}, "NotEqual"},
		y:     &ts.StructD2{&ts.StructD{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructE2",
		x:     ts.StructE2{&ts.StructE{"NotEqual"}, "NotEqual"},
		y:     ts.StructE2{&ts.StructE{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructE2",
		x:     &ts.StructE2{&ts.StructE{"NotEqual"}, "NotEqual"},
		y:     &ts.StructE2{&ts.StructE{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructF2",
		x:     ts.StructF2{&ts.StructF{"NotEqual"}, "NotEqual"},
		y:     ts.StructF2{&ts.StructF{"not_equal"}, "not_equal"},
	}, {
		label: label + "StructF2",
		x:     &ts.StructF2{&ts.StructF{"NotEqual"}, "NotEqual"},
		y:     &ts.StructF2{&ts.StructF{"not_equal"}, "not_equal"},
	}, {
		label:    label + "StructNo",
		x:        ts.StructNo{"NotEqual"},
		y:        ts.StructNo{"not_equal"},
		wantDiff: "{teststructs.StructNo}.X:\n\t-: \"NotEqual\"\n\t+: \"not_equal\"\n",
	}, {
		label: label + "AssignA",
		x:     ts.AssignA(func() int { return 0 }),
		y:     ts.AssignA(func() int { return 1 }),
	}, {
		label: label + "AssignB",
		x:     ts.AssignB(struct{ A int }{0}),
		y:     ts.AssignB(struct{ A int }{1}),
	}, {
		label: label + "AssignC",
		x:     ts.AssignC(make(chan bool)),
		y:     ts.AssignC(make(chan bool)),
	}, {
		label: label + "AssignD",
		x:     ts.AssignD(make(chan bool)),
		y:     ts.AssignD(make(chan bool)),
	}}
}

func project1Tests() []test {
	const label = "Project1"

	ignoreUnexported := IgnoreUnexported(
		ts.EagleImmutable{},
		ts.DreamerImmutable{},
		ts.SlapImmutable{},
		ts.GoatImmutable{},
		ts.DonkeyImmutable{},
		ts.LoveRadius{},
		ts.SummerLove{},
		ts.SummerLoveSummary{},
	)

	createEagle := func() ts.Eagle {
		return ts.Eagle{
			Name:   "eagle",
			Hounds: []string{"buford", "tannen"},
			Desc:   "some description",
			Dreamers: []ts.Dreamer{{}, {
				Name: "dreamer2",
				Animal: []interface{}{
					ts.Goat{
						Target: "corporation",
						Immutable: &ts.GoatImmutable{
							ID:      "southbay",
							State:   (*pb.Goat_States)(intPtr(5)),
							Started: now,
						},
					},
					ts.Donkey{},
				},
				Amoeba: 53,
			}},
			Slaps: []ts.Slap{{
				Name: "slapID",
				Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
				Immutable: &ts.SlapImmutable{
					ID:       "immutableSlap",
					MildSlap: true,
					Started:  now,
					LoveRadius: &ts.LoveRadius{
						Summer: &ts.SummerLove{
							Summary: &ts.SummerLoveSummary{
								Devices:    []string{"foo", "bar", "baz"},
								ChangeType: []pb.SummerType{1, 2, 3},
							},
						},
					},
				},
			}},
			Immutable: &ts.EagleImmutable{
				ID:          "eagleID",
				Birthday:    now,
				MissingCall: (*pb.Eagle_MissingCalls)(intPtr(55)),
			},
		}
	}

	return []test{{
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
		}}},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
		}}},
		opts:     []cmp.Option{cmp.Comparer(pb.Equal)},
		wantDiff: "",
	}, {
		label: label,
		x: ts.Eagle{Slaps: []ts.Slap{{}, {}, {}, {}, {
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata"}},
		}}},
		y: ts.Eagle{Slaps: []ts.Slap{{}, {}, {}, {}, {
			Args: &pb.MetaData{Stringer: pb.Stringer{"metadata2"}},
		}}},
		opts:     []cmp.Option{cmp.Comparer(pb.Equal)},
		wantDiff: "{teststructs.Eagle}.Slaps[4].Args:\n\t-: \"metadata\"\n\t+: \"metadata2\"\n",
	}, {
		label: label,
		x:     createEagle(),
		y:     createEagle(),
		opts:  []cmp.Option{ignoreUnexported, cmp.Comparer(pb.Equal)},
	}, {
		label: label,
		x: func() ts.Eagle {
			eg := createEagle()
			eg.Dreamers[1].Animal[0].(ts.Goat).Immutable.ID = "southbay2"
			eg.Dreamers[1].Animal[0].(ts.Goat).Immutable.State = (*pb.Goat_States)(intPtr(6))
			eg.Slaps[0].Immutable.MildSlap = false
			return eg
		}(),
		y: func() ts.Eagle {
			eg := createEagle()
			devs := eg.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices
			eg.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices = devs[:1]
			return eg
		}(),
		opts: []cmp.Option{ignoreUnexported, cmp.Comparer(pb.Equal)},
		wantDiff: `
{teststructs.Eagle}.Dreamers[1].Animal[0].(teststructs.Goat).Immutable.ID:
	-: "southbay2"
	+: "southbay"
*{teststructs.Eagle}.Dreamers[1].Animal[0].(teststructs.Goat).Immutable.State:
	-: 6
	+: 5
{teststructs.Eagle}.Slaps[0].Immutable.MildSlap:
	-: false
	+: true
{teststructs.Eagle}.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices[1]:
	-: "bar"
	+: <non-existent>
{teststructs.Eagle}.Slaps[0].Immutable.LoveRadius.Summer.Summary.Devices[2]:
	-: "baz"
	+: <non-existent>`,
	}}
}

func project2Tests() []test {
	const label = "Project2"

	sortGerms := cmp.FilterValues(func(x, y []*pb.Germ) bool {
		ok1 := sort.SliceIsSorted(x, func(i, j int) bool {
			return x[i].String() < x[j].String()
		})
		ok2 := sort.SliceIsSorted(y, func(i, j int) bool {
			return y[i].String() < y[j].String()
		})
		return !ok1 || !ok2
	}, cmp.Transformer("Sort", func(in []*pb.Germ) []*pb.Germ {
		out := append([]*pb.Germ(nil), in...) // Make copy
		sort.Slice(out, func(i, j int) bool {
			return out[i].String() < out[j].String()
		})
		return out
	}))

	equalDish := cmp.Comparer(func(x, y *ts.Dish) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		px, err1 := x.Proto()
		py, err2 := y.Proto()
		if err1 != nil || err2 != nil {
			return err1 == err2
		}
		return pb.Equal(px, py)
	})

	createBatch := func() ts.GermBatch {
		return ts.GermBatch{
			DirtyGerms: map[int32][]*pb.Germ{
				17: []*pb.Germ{
					&pb.Germ{Stringer: pb.Stringer{"germ1"}},
				},
				18: []*pb.Germ{
					&pb.Germ{Stringer: pb.Stringer{"germ2"}},
					&pb.Germ{Stringer: pb.Stringer{"germ3"}},
					&pb.Germ{Stringer: pb.Stringer{"germ4"}},
				},
			},
			GermMap: map[int32]*pb.Germ{
				13: &pb.Germ{Stringer: pb.Stringer{"germ13"}},
				21: &pb.Germ{Stringer: pb.Stringer{"germ21"}},
			},
			DishMap: map[int32]*ts.Dish{
				0: ts.CreateDish(nil, io.EOF),
				1: ts.CreateDish(nil, io.ErrUnexpectedEOF),
				2: ts.CreateDish(&pb.Dish{Stringer: pb.Stringer{"dish"}}, nil),
			},
			HasPreviousResult: true,
			DirtyID:           10,
			GermStrain:        421,
			InfectedAt:        now,
		}
	}

	return []test{{
		label:     label,
		x:         createBatch(),
		y:         createBatch(),
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createBatch(),
		y:     createBatch(),
		opts:  []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
	}, {
		label: label,
		x:     createBatch(),
		y: func() ts.GermBatch {
			gb := createBatch()
			s := gb.DirtyGerms[18]
			s[0], s[1], s[2] = s[1], s[2], s[0]
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), equalDish},
		wantDiff: `
{teststructs.GermBatch}.DirtyGerms[18][0]:
	-: "germ2"
	+: "germ3"
{teststructs.GermBatch}.DirtyGerms[18][1]:
	-: "germ3"
	+: "germ4"
{teststructs.GermBatch}.DirtyGerms[18][2]:
	-: "germ4"
	+: "germ2"`,
	}, {
		label: label,
		x:     createBatch(),
		y: func() ts.GermBatch {
			gb := createBatch()
			s := gb.DirtyGerms[18]
			s[0], s[1], s[2] = s[1], s[2], s[0]
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
	}, {
		label: label,
		x: func() ts.GermBatch {
			gb := createBatch()
			delete(gb.DirtyGerms, 17)
			gb.DishMap[1] = nil
			return gb
		}(),
		y: func() ts.GermBatch {
			gb := createBatch()
			gb.DirtyGerms[18] = gb.DirtyGerms[18][:2]
			gb.GermStrain = 22
			return gb
		}(),
		opts: []cmp.Option{cmp.Comparer(pb.Equal), sortGerms, equalDish},
		wantDiff: `
{teststructs.GermBatch}.DirtyGerms[17]:
	-: <non-existent>
	+: []*testprotos.Germ{"germ1"}
{teststructs.GermBatch}.DirtyGerms[18][2]:
	-: "germ4"
	+: <non-existent>
{teststructs.GermBatch}.DishMap[1]:
	-: (*teststructs.Dish)(nil)
	+: &teststructs.Dish{err: &errors.errorString{s: "unexpected EOF"}}
{teststructs.GermBatch}.GermStrain:
	-: 421
	+: 22`,
	}}
}

func project3Tests() []test {
	const label = "Project3"

	allowVisibility := cmp.AllowUnexported(ts.Dirt{})

	ignoreLocker := cmp.FilterPath(func(p cmp.Path) bool {
		return len(p) > 0 && p[len(p)-1].Type() == mutexType
	}, cmp.Ignore())

	transformProtos := cmp.Transformer("", func(x pb.Dirt) *pb.Dirt {
		return &x
	})

	equalTable := cmp.Comparer(func(x, y ts.Table) bool {
		tx, ok1 := x.(*ts.MockTable)
		ty, ok2 := y.(*ts.MockTable)
		if !ok1 || !ok2 {
			panic("table type must be MockTable")
		}
		return cmp.Equal(tx.State(), ty.State())
	})

	createDirt := func() (d ts.Dirt) {
		d.SetTable(ts.CreateMockTable([]string{"a", "b", "c"}))
		d.SetTimestamp(12345)
		d.Discord = 554
		d.Proto = pb.Dirt{Stringer: pb.Stringer{"proto"}}
		d.SetWizard(map[string]*pb.Wizard{
			"harry": &pb.Wizard{Stringer: pb.Stringer{"potter"}},
			"albus": &pb.Wizard{Stringer: pb.Stringer{"dumbledore"}},
		})
		d.SetLastTime(54321)
		return d
	}

	return []test{{
		label:     label,
		x:         createDirt(),
		y:         createDirt(),
		wantPanic: "cannot handle unexported field",
	}, {
		label:     label,
		x:         createDirt(),
		y:         createDirt(),
		opts:      []cmp.Option{allowVisibility, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createDirt(),
		y:     createDirt(),
		opts:  []cmp.Option{allowVisibility, transformProtos, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
	}, {
		label: label,
		x: func() ts.Dirt {
			d := createDirt()
			d.SetTable(ts.CreateMockTable([]string{"a", "c"}))
			d.Proto = pb.Dirt{Stringer: pb.Stringer{"blah"}}
			return d
		}(),
		y: func() ts.Dirt {
			d := createDirt()
			d.Discord = 500
			d.SetWizard(map[string]*pb.Wizard{
				"harry": &pb.Wizard{Stringer: pb.Stringer{"otter"}},
			})
			return d
		}(),
		opts: []cmp.Option{allowVisibility, transformProtos, ignoreLocker, cmp.Comparer(pb.Equal), equalTable},
		wantDiff: `
{teststructs.Dirt}.table:
	-: &teststructs.MockTable{state: []string{"a", "c"}}
	+: &teststructs.MockTable{state: []string{"a", "b", "c"}}
{teststructs.Dirt}.Discord:
	-: 554
	+: 500
λ({teststructs.Dirt}.Proto):
	-: "blah"
	+: "proto"
{teststructs.Dirt}.wizard["albus"]:
	-: "dumbledore"
	+: <non-existent>
{teststructs.Dirt}.wizard["harry"]:
	-: "potter"
	+: "otter"`,
	}}
}

func project4Tests() []test {
	const label = "Project4"

	allowVisibility := cmp.AllowUnexported(
		ts.Cartel{},
		ts.Headquarter{},
		ts.Poison{},
	)

	transformProtos := cmp.Transformer("", func(x pb.Restrictions) *pb.Restrictions {
		return &x
	})

	createCartel := func() ts.Cartel {
		var p ts.Poison
		p.SetPoisonType(5)
		p.SetExpiration(now)
		p.SetManufactuer("acme")

		var hq ts.Headquarter
		hq.SetID(5)
		hq.SetLocation("moon")
		hq.SetSubDivisions([]string{"alpha", "bravo", "charlie"})
		hq.SetMetaData(&pb.MetaData{Stringer: pb.Stringer{"metadata"}})
		hq.SetPublicMessage([]byte{1, 2, 3, 4, 5})
		hq.SetHorseBack("abcdef")
		hq.SetStatus(44)

		var c ts.Cartel
		c.Headquarter = hq
		c.SetSource("mars")
		c.SetCreationTime(now)
		c.SetBoss("al capone")
		c.SetPoisons([]*ts.Poison{&p})

		return c
	}

	return []test{{
		label:     label,
		x:         createCartel(),
		y:         createCartel(),
		wantPanic: "cannot handle unexported field",
	}, {
		label:     label,
		x:         createCartel(),
		y:         createCartel(),
		opts:      []cmp.Option{allowVisibility, cmp.Comparer(pb.Equal)},
		wantPanic: "cannot handle unexported field",
	}, {
		label: label,
		x:     createCartel(),
		y:     createCartel(),
		opts:  []cmp.Option{allowVisibility, transformProtos, cmp.Comparer(pb.Equal)},
	}, {
		label: label,
		x: func() ts.Cartel {
			d := createCartel()
			var p1, p2 ts.Poison
			p1.SetPoisonType(1)
			p1.SetExpiration(now)
			p1.SetManufactuer("acme")
			p2.SetPoisonType(2)
			p2.SetManufactuer("acme2")
			d.SetPoisons([]*ts.Poison{&p1, &p2})
			return d
		}(),
		y: func() ts.Cartel {
			d := createCartel()
			d.SetSubDivisions([]string{"bravo", "charlie"})
			d.SetPublicMessage([]byte{1, 2, 4, 3, 5})
			return d
		}(),
		opts: []cmp.Option{allowVisibility, transformProtos, cmp.Comparer(pb.Equal)},
		wantDiff: `
{teststructs.Cartel}.Headquarter.subDivisions[0]:
	-: "alpha"
	+: "bravo"
{teststructs.Cartel}.Headquarter.subDivisions[1]:
	-: "bravo"
	+: "charlie"
{teststructs.Cartel}.Headquarter.subDivisions[2]:
	-: "charlie"
	+: <non-existent>
{teststructs.Cartel}.Headquarter.publicMessage[2]:
	-: 0x03
	+: 0x04
{teststructs.Cartel}.Headquarter.publicMessage[3]:
	-: 0x04
	+: 0x03
{teststructs.Cartel}.poisons[0].poisonType:
	-: 1
	+: 5
{teststructs.Cartel}.poisons[1]:
	-: &teststructs.Poison{poisonType: 2, manufactuer: "acme2"}
	+: <non-existent>`,
	}}
}
