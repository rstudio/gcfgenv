// Copyright 2026 Posit Software, PBC
// SPDX-License-Identifier: Apache-2.0

package gcfgenv

// A differential test against gcfg itself. This package's contract is that an
// environment variable and a configuration file entry are equivalent, so the
// only trustworthy oracle for a name is what gcfg does with it.
//
// Candidate names are generated from reflection over the struct shape, not from
// flatFields, so the test cannot inherit the blind spots of the code it checks.
// For every name gcfg accepts, the environment must produce the same result; for
// every name gcfg refuses, the environment must not quietly set something.

import (
	"reflect"
	"strings"

	"gopkg.in/check.v1"
	"gopkg.in/gcfg.v1"
)

// Types used to build the shapes below. Exported and unexported variants matter:
// reflect will not set an unexported embedded field, and gcfg skips what it
// cannot set.
type parityInner struct {
	Alpha string
	Beta  []string
}

type parityInnerUnexported struct {
	Alpha string
	Gamma string
}

type parityTagged struct {
	Delta string `gcfg:"delta-name"`
}

type parityDeep struct {
	parityInner
	Epsilon string
}

type paritySub struct {
	parityInner
	Alpha string
}

type parityCollide struct {
	parityInner
	ParityInner string
}

type parityShadowed struct {
	parityInner
	Alpha string
	Zeta  string
}

type paritySection struct {
	Level string
	Items []string
}

type parityEmbeddedSections struct {
	Logging paritySection
}

// parityTagOptions exercises a gcfg tag carrying options, which both names the
// field and changes how its value is parsed.
type parityTagOptions struct {
	Numbered int `gcfg:"the-number,int=dho"`
	Optioned int `gcfg:",int=dho"`
	Plain    int
}

type parityTagOptionsSub struct {
	URL      string
	Numbered int `gcfg:"the-number,int=dho"`
}

// parityShapes are whole configuration structs, each exercising one shape.
var parityShapes = []interface{}{
	// A plain section with an exported embedded struct.
	&struct {
		Sec struct {
			parityInner
			Other string
		}
	}{},
	// An unexported embedded struct, reachable only through its fields.
	&struct {
		Sec struct {
			parityInnerUnexported
			Other string
		}
	}{},
	// An embedded field whose type name collides with a sibling field.
	&struct{ Sec parityCollide }{},
	// A sibling that shadows a promoted field.
	&struct{ Sec parityShadowed }{},
	// A gcfg tag on a promoted field.
	&struct {
		Sec struct {
			parityTagged
			Other string
		}
	}{},
	// Two levels of embedding.
	&struct {
		Sec struct {
			parityDeep
			Other string
		}
	}{},
	// Subsections, with and without embedding.
	&struct {
		Sec map[string]*paritySub
	}{},
	&struct {
		Sec map[string]*paritySection
	}{},
	// Sections promoted from an embedded struct.
	&struct {
		parityEmbeddedSections
		Server paritySection
	}{},
	// A section name colliding with an embedded struct's type name.
	&struct {
		parityEmbeddedSections
		ParityEmbeddedSections paritySection
	}{},
	// gcfg tags carrying options, in a section and in a subsection.
	&struct{ Sec parityTagOptions }{},
	&struct {
		Sec map[string]*parityTagOptionsSub
	}{},
}

// parityValues are tried in order; the first one gcfg accepts is the one
// compared. Covering several kinds keeps a name from being judged unreachable
// merely because the value did not suit its type.
var parityValues = []string{"parityvalue", "7", "true"}

// parityKeys are the subsection names to try for a map-valued section. One of
// them ends in a property name, which is how a key gets mangled if the suffix is
// trimmed carelessly.
var parityKeys = []string{"k1", "PROD_URL_v2"}

func (s *Suite) TestParityWithGcfgAcrossShapes(c *check.C) {
	checked := 0
	for _, shape := range parityShapes {
		t := reflect.TypeOf(shape).Elem()
		cands := parityCandidates(t)
		// A guard on the guard: a shape that generates no names would
		// pass vacuously.
		c.Check(len(cands) > 0, check.Equals, true,
			check.Commentf("%s generated no candidate names", t))
		for _, cand := range cands {
			checked++
			parityCheckOne(c, shape, cand)
		}
	}
	c.Check(checked > 30, check.Equals, true,
		check.Commentf("only %d names generated; the candidate walk is broken", checked))
	c.Logf("compared %d candidate names against gcfg", checked)
}

// candidate is one name to try, in both notations.
type candidate struct {
	section  string
	subKey   string
	property string
}

func (cand candidate) fileText(value string) string {
	header := "[" + cand.section + "]"
	if cand.subKey != "" {
		header = "[" + cand.section + " \"" + cand.subKey + "\"]"
	}
	return header + "\n" + cand.property + " = " + value + "\n"
}

// envVar restates the documented naming rules independently of
// fieldToEnvVar, which is part of what is under test: uppercase, and dashes
// become underscores. Subsection names are left alone.
func (cand candidate) envVar() string {
	name := func(s string) string {
		return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
	}
	parts := []string{name(cand.section)}
	if cand.subKey != "" {
		parts = append(parts, cand.subKey)
	}
	parts = append(parts, name(cand.property))
	return strings.Join(parts, "_")
}

// parityCandidates lists every name that could plausibly address something in t:
// every field name at every depth, plus every embedded type name, on both the
// section and the property axis. Deliberately generated without consulting
// flatFields.
func parityCandidates(t reflect.Type) []candidate {
	var out []candidate
	for _, sec := range parityNames(t) {
		sf, ok := parityFieldByName(t, sec)
		if !ok {
			continue
		}
		switch sf.Type.Kind() {
		case reflect.Struct:
			for _, prop := range parityNames(sf.Type) {
				out = append(out, candidate{section: sec, property: prop})
			}
		case reflect.Map:
			elem := sf.Type.Elem()
			if elem.Kind() != reflect.Ptr || elem.Elem().Kind() != reflect.Struct {
				continue
			}
			for _, prop := range parityNames(elem.Elem()) {
				for _, key := range parityKeys {
					out = append(out, candidate{section: sec, subKey: key, property: prop})
				}
			}
		}
	}
	return out
}

// parityNames returns the syntactic names of a struct: its own fields, the type
// names of its embedded structs, and recursively the names those embed.
func parityNames(t reflect.Type) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	var walk func(reflect.Type)
	walk = func(st reflect.Type) {
		for i := 0; i < st.NumField(); i++ {
			sf := st.Field(i)
			// Only the part before the first comma names the field;
			// the rest are options such as `int=dho`.
			if tag, _, _ := strings.Cut(sf.Tag.Get("gcfg"), ","); tag != "" {
				add(tag)
			}
			add(sf.Name)
			if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
				walk(sf.Type)
			}
		}
	}
	walk(t)
	return out
}

// parityFieldByName resolves a name to a field the way the candidate generator
// needs it, tolerating unexported and embedded names.
func parityFieldByName(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Name == name || sf.Tag.Get("gcfg") == name {
			return sf, true
		}
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			if inner, ok := parityFieldByName(sf.Type, name); ok {
				return inner, true
			}
		}
	}
	return reflect.StructField{}, false
}

func parityCheckOne(c *check.C, shape interface{}, cand candidate) {
	shapeType := reflect.TypeOf(shape).Elem()

	for _, value := range parityValues {
		fromFile := reflect.New(shapeType)
		fileErr := gcfg.ReadStringInto(fromFile.Interface(), cand.fileText(value))

		fromEnv := reflect.New(shapeType)
		envErr := readWithMapInto(strings.NewReader(""),
			map[string]string{cand.envVar(): value}, "", fromEnv.Interface())

		zero := reflect.New(shapeType)

		if fileErr == nil {
			// gcfg accepted this name, so the environment must reach
			// the same field with the same result.
			c.Check(envErr, check.IsNil, check.Commentf(
				"%s: %s=%s errored but the file form did not", shapeType, cand.envVar(), value))
			c.Check(fromEnv.Elem().Interface(), check.DeepEquals, fromFile.Elem().Interface(),
				check.Commentf("%s: %s=%s did not match the file form %q",
					shapeType, cand.envVar(), value, strings.TrimSpace(cand.fileText(value))))
			return
		}

		// gcfg refused this name. The environment may refuse it too, but
		// it must not silently set something.
		if envErr == nil {
			c.Check(fromEnv.Elem().Interface(), check.DeepEquals, zero.Elem().Interface(),
				check.Commentf("%s: %s=%s set a field that the file form refuses (%v)",
					shapeType, cand.envVar(), value, fileErr))
		}
	}
}
