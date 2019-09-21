// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright 2019, TIBCO Software Inc. This file is subject to the license
// terms contained in the license file that is distributed with this file.

// Purpose built command line tool to generate the desired enumerations
// necessary for CVRF and JSON format documents.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"os"
	"text/template"
)

type value struct {
	GoName string
	XML    string
	JSON   string
}

type enum struct {
	TypeName string
	Comment  string
	Values   []*value
}

type jobDef struct {
	PackageName string
	Enums       []*enum
}

type config struct {
	definitions string
	destination string
	help        bool
}

func parseArgs(appName string, args []string) (*config, error) {

	var result config
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.StringVar(&result.destination, "destination", "enums.go",
		"file name for the generated output")
	fs.StringVar(&result.definitions, "definitions", "",
		"path to JSON format file containing enumeration definitions.")
	fs.BoolVar(&result.help, "help", false, "shows the help message and exits")

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}
	if result.destination == "" {
		return nil, fmt.Errorf("no destination specified")
	}

	return &result, nil
}

const helpMsg = `
Generates a Go source file containing enumerations.

The -destination option indicates the target source file to generate.

The -definitions option indicates the path to a JSON file containing the
definitions of enumerations. Sample:

{
	"PackageName": "mypkg",
	"Enums": [
		{
			"TypeName": "EnumType",
			"Values": [
				{
					"GoName": "BlahValue",
					"XML": "xml-blah-value",
					"JSON": "json-blah-value"
				},
				{
					"GoName": "GoodValue",
					"XML": "xml-good-value",
					"JSON": "json-good-value"
				}
			]
		}
	]
}
`

const pkgTemplate = `// Code generated by genenums - DO NOT EDIT
//
// This file implements marshaling and unmarshalling to JSON and XML for the
// purposes of having types with enumerated values - that serialize to different
// values in XML vs. JSON.
//
// Types and their values are defined in a JSON file, and provided to the
// genenums tool.

package {{ .PackageName }}

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

func xmlElemAsString(d *xml.Decoder, start xml.StartElement) (string, error) {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return "", err
	}
	return s, nil
}

func noQuotes(b []byte) string {
	return strings.TrimSuffix(strings.TrimPrefix(string(b), "\""), "\"")
}

{{range .Enums}}
{{$typeName := .TypeName -}}
/*******************************************************************************
* Generated type {{.TypeName}}
*******************************************************************************/

// {{.Comment}}
type {{.TypeName}} int

// Values for {{.TypeName}}
const (
{{range $idx, $enum := .Values}}	{{.GoName}}
	{{- if eq $idx 0}} {{$typeName}} = iota{{end}}
{{end}}
	max{{.TypeName}}
)

var gen{{.TypeName}}ToXMLStr = map[{{.TypeName}}]string{
{{range .Values}}	{{.GoName}}: "{{.XML}}",
{{end -}}
}

var genXMLStrTo{{.TypeName}} = map[string]{{.TypeName}}{
{{range .Values}}	"{{.XML}}": {{.GoName}},
{{end -}}
}

// UnmarshalXML implemented to support reading from XML.
func(obj *{{.TypeName}}) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s, err := xmlElemAsString(d, start)
	if err != nil {
		return fmt.Errorf("problem decoding {{.TypeName}}: %v", err)
	}
	var ok bool
	if *obj, ok = genXMLStrTo{{.TypeName}}[s]; !ok {
		return fmt.Errorf("unrecognized {{.TypeName}} value %v", s)
	}
	return nil
}

func (obj *{{.TypeName}}) mapXMLValue(s string) error {
	var ok bool
	if *obj, ok = genXMLStrTo{{.TypeName}}[s]; !ok {
		return fmt.Errorf("unrecognized {{.TypeName}} value %v", s)
	}
	return nil
}

// UnmarshalXMLAttr implemented to support reading from XML
func (obj *{{.TypeName}}) UnmarshalXMLAttr(attr xml.Attr) error {
	return obj.mapXMLValue(attr.Value)
}

// MarshalXMLAttr implemented to support writing to XML.
func (obj {{.TypeName}}) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	s, ok := gen{{.TypeName}}ToXMLStr[obj]
	if !ok {
		return xml.Attr{}, fmt.Errorf("unrecognized {{.TypeName}} value %v", obj)
	}
	return xml.Attr{Name: name, Value: s}, nil
}

// MarshalXML implemented to support writing to XML.
func (obj {{.TypeName}}) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	s, ok := gen{{.TypeName}}ToXMLStr[obj]
	if !ok {
		return fmt.Errorf("unrecognized {{.TypeName}} %v", obj)
	}
	return e.EncodeElement(s, start)
}

var genJSONStrTo{{.TypeName}} = map[string]{{.TypeName}}{
	{{range .Values}}	"{{.JSON}}": {{.GoName}},
	{{end -}}
}
	
var gen{{.TypeName}}ToJSONStr = map[{{.TypeName}}]string{
	{{range .Values}}	{{.GoName}}: "{{.JSON}}",
	{{end -}}
}

// UnmarshalJSON implemented to support writing to XML.
func (obj *{{.TypeName}}) UnmarshalJSON(data []byte) error {
	var ok bool
	if *obj, ok = genJSONStrTo{{.TypeName}}[noQuotes(data)]; !ok {
		return fmt.Errorf("unrecognized {{.TypeName}} value %v", string(data))
	}
	return nil
}

// MarshalJSON implemented to support writing to XML.
func (obj {{.TypeName}}) MarshalJSON() ([]byte, error) {
	s, ok := gen{{.TypeName}}ToJSONStr[obj]
	if !ok {
		return nil, fmt.Errorf("unrecognized {{.TypeName}} value %v", obj)
	}
	return json.Marshal(s)
}

func (obj {{.TypeName}}) check(val *Validator) {
	if obj < 0 || obj >= max{{.TypeName}} {
		val.err(fmt.Sprintf("unrecognized {{.TypeName}} %v", obj))
	}
}
{{end}}
`

func (j jobDef) generate(w io.Writer) error {

	tmpl, err := template.New("codegen.go").Parse(pkgTemplate)
	if err != nil {
		return fmt.Errorf("unable to parse generator template: %v", err)
	}

	return tmpl.Execute(w, j)
}

// check() verifies the job definition has valid input values.
func (j jobDef) check() error {
	if j.PackageName == "" {
		return fmt.Errorf("empty package name not allowed")
	}
	for _, enum := range j.Enums {
		if enum.TypeName == "" {
			return fmt.Errorf("empty type name not allowed")
		}
		if len(enum.Values) == 0 {
			return fmt.Errorf("must have values specified for type %v", enum.TypeName)
		}
		for _, val := range enum.Values {
			if val.GoName == "" {
				return fmt.Errorf("empty Go name specified for type %v", enum.TypeName)
			}
			if val.JSON == "" {
				return fmt.Errorf("empty JSON value specified for type %v", enum.TypeName)
			}
			if val.XML == "" {
				return fmt.Errorf("empty XML value specified for type %v", enum.TypeName)
			}
		}
	}

	return nil
}

func run(appName string, args []string) error {
	cfg, err := parseArgs(appName, args)
	if err != nil {
		return err
	}

	if cfg.help {
		fmt.Print(helpMsg)
		return nil
	}

	raw, err := ioutil.ReadFile(cfg.definitions)
	if err != nil {
		return fmt.Errorf("problem reading definitions JSON file: %v", err)
	}
	var job jobDef

	err = json.Unmarshal(raw, &job)
	if err != nil {
		return fmt.Errorf("problem parsing definitions JSON file: %v", err)
	}

	if err = job.check(); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err = job.generate(&buf); err != nil {
		return fmt.Errorf("problem generating source: %v", err)
	}
	toSave, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("unable to format go code: %v", err)
	}
	out, err := os.Create(cfg.destination)
	if err != nil {
		return fmt.Errorf("problem opening output file: %v", err)
	}
	defer safeClose(out)
	if _, err = out.Write(toSave); err != nil {
		return fmt.Errorf("difficulty writing file: %v", err)
	}
	return nil
}

func safeClose(rc io.Closer) {
	rc.Close() //nolint:errcheck,gosec
}

func main() {

	err := run(os.Args[0], os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	os.Exit(0)
}
