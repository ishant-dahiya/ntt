package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"html/template"
	"log"
	"os"
	"sort"
	"strings"
)

var tmpl = `// Code generated by go generate; DO NOT EDIT.

package syntax

import "github.com/nokia/ntt/internal/loc"

{{ range $name, $type := . }}

{{ if ($type.NotImplemented "FirstTok" ) }}
func (n *{{ $name }}) FirstTok() Token {
	switch {
	{{ range $i, $field := $type.Fields }}
	{{ if $field.IsArray }}case len(n.{{ $field.Name }}) > 0:
		return n.{{ $field.Name }}[0].FirstTok()
	{{ else }}case n.{{ $field.Name }} != nil:
		return n.{{ $field.Name }}{{ if eq $field.IsToken false }}.FirstTok(){{ end }}
	{{ end }}
	{{ end }}
	default:
		return nil
	}
}
{{ end }}

{{ if $type.NotImplemented "LastTok" }}
func (n *{{ $name }}) LastTok() Token {
	switch {
	{{ range $i, $f := $type.Fields }}
	{{ $field := index $type.Fields (sub (len $type.Fields) (add $i 1)) }}
	{{ if $field.IsArray }}case len(n.{{ $field.Name }}) > 0:
		return n.{{ $field.Name }}[len(n.{{$field.Name}})-1].LastTok()
	{{ else }}case n.{{ $field.Name }} != nil:
		return n.{{ $field.Name }}{{ if eq $field.IsToken false }}.LastTok(){{ end }}
	{{ end }}
	{{ end }}
	default:
		return nil
	}
}
{{ end }}

{{ if $type.NotImplemented "Children" }}
func (n *{{ $name }}) Children() []Node {
	ret := make([]Node, 0, {{ len $type.Fields }})
	{{ range $i, $field := $type.Fields }}
	{{ if $field.IsArray }}
	for _, c := range n.{{ $field.Name }} {
		ret = append(ret, c)
	}
	{{ else }}
	if n.{{ $field.Name }} != nil {
		ret = append(ret, n.{{ $field.Name }})
	}
	{{ end }}
	{{ end }}
	return ret
}
{{ end }}

{{ if $type.NotImplemented "Inspect" }}
func (n *{{ $name }}) Inspect(f func(Node) bool) {
	if !f(n) {
		return
	}
	{{ range $i, $field := $type.Fields }}
	{{ if $field.IsArray }}
	for _, c := range n.{{ $field.Name }} {
		c.Inspect(f)
	}
	{{ else if eq $field.IsToken false }}
	if n.{{ $field.Name }} != nil {
		n.{{ $field.Name }}.Inspect(f)
	}
	{{ end }}
	{{ end }}
	f(nil)
}
{{ end }}

{{ if $type.NotImplemented "Pos" }}
func (n *{{ $name }}) Pos() loc.Pos {
	if tok := n.FirstTok(); tok != nil {
		return tok.Pos()
	}
	return loc.NoPos
}
{{ end }}

{{ if $type.NotImplemented "End" }}
func (n *{{ $name }}) End() loc.Pos {
	if tok := n.LastTok(); tok != nil {
		return tok.End()
	}
	return loc.NoPos
}
{{ end }}

{{ end }}
`

type Type struct {
	Name    string
	Fields  []Field
	Methods []string
}

func (f Type) NotImplemented(name string) bool {
	for _, m := range f.Methods {
		if m == name {
			return false
		}
	}
	return true
}

type Field struct {
	Name string
	Type string
}

func (f *Field) IsArray() bool {
	return strings.HasPrefix(f.Type, "[]")
}

func (f *Field) IsToken() bool {
	return strings.HasPrefix(f.Type, "Token")
}

func Fields(fset *token.FileSet, list []*ast.Field) []Field {
	var fields []Field
	for _, field := range list {
		if len(field.Names) == 0 {
			name := strings.TrimLeft(typeString(fset, field.Type), "*[]")
			field.Names = []*ast.Ident{&ast.Ident{Name: name}}
		}
		for _, fieldName := range field.Names {
			if !isLocalType(field.Type) {
				continue
			}

			fields = append(fields, Field{
				Name: fieldName.Name,
				Type: typeString(fset, field.Type),
			})
		}
	}
	return fields
}

func isLocalType(x ast.Expr) bool {
	ret := true
	ast.Inspect(x, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			if n.Obj == nil {
				ret = false
				return false
			}
		}
		return true
	})
	return ret
}

func typeString(fset *token.FileSet, x ast.Expr) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, fset, x)
	return buf.String()
}

func main() {
	fset := token.NewFileSet()
	src, err := parser.ParseFile(fset, "ast.go", nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		log.Fatal(err.Error())
	}

	typesMap := make(map[string]Type)
	ast.Inspect(src, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeSpec:
			name := n.Name.Name
			st, ok := n.Type.(*ast.StructType)
			if !ok {
				return false
			}
			typ, ok := typesMap[name]
			if !ok {
				typ = Type{Name: name}
			}
			typ.Fields = append(typ.Fields, Fields(fset, st.Fields.List)...)
			typesMap[name] = typ
			return false

		case *ast.FuncDecl:
			if n.Recv == nil || len(n.Recv.List) != 1 {
				return false
			}
			recv := typeString(fset, n.Recv.List[0].Type)
			typ, ok := typesMap[recv]
			if !ok {
				return false
			}
			typ.Methods = append(typ.Methods, n.Name.Name)
			typesMap[recv] = typ
			return false
		default:
			return true
		}
	})

	out, err := os.Create("ast_gen.go")
	if err != nil {
		log.Fatal(err.Error())
	}
	defer out.Close()

	t, err := template.New("ast_gen.go").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}).Parse(tmpl)
	if err != nil {
		log.Fatal(err)
	}

	types := make([]Type, 0, len(typesMap))
	for _, typ := range typesMap {
		types = append(types, typ)
	}
	sort.Slice(types, func(i, j int) bool {
		return types[i].Name < types[j].Name
	})

	buf := bytes.NewBuffer(nil)
	err = t.Execute(buf, typesMap)
	if err != nil {
		log.Fatal(err)
	}
	ans, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	out.Write(ans)
}