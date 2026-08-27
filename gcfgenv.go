// Copyright 2024 Posit Software, PBC
// SPDX-License-Identifier: Apache-2.0

// Package gcfgenv allows reading gcfg configurations (see
// https://gopkg.in/gcfg.v1) that respect overrides specified in environment
// variables.
package gcfgenv

import (
	"bytes"
	"encoding"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"gopkg.in/gcfg.v1"
	"gopkg.in/gcfg.v1/types"
)

// ReadFileWithEnvInto reads the gcfg-formatted file at filename, injects any
// overrides from the process's environment variables (prefixed with envPrefix),
// and sets these values in the corresponding fields of config.
func ReadFileWithEnvInto(filename string, envPrefix string, config interface{}) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	maybeSkipBOM(f)
	return ReadWithEnvInto(f, envPrefix, config)
}

// ReadWithEnvInto reads gcfg-formatted data from r, injects any overrides from
// the process's environment variables (prefixed with envPrefix), and sets these
// values in the corresponding fields of config.
func ReadWithEnvInto(r io.Reader, envPrefix string, config interface{}) error {
	env := mapFromEnviron(os.Environ())
	return readWithMapInto(r, env, envPrefix, config)
}

var utf8BOM = []byte("\ufeff")

func maybeSkipBOM(r io.ReadSeeker) {
	b := make([]byte, len(utf8BOM))
	read, err := r.Read(b)
	if err == nil && read == len(utf8BOM) {
		// If we don't find a BOM, we need to seek back over the bytes
		// we've read.
		if !bytes.Equal(b, utf8BOM) {
			// We can ignore errors here, they will resurface later
			// when reading.
			r.Seek(int64(-read), 1)
		}
		return
	}
	// We can ignore errors here, they will resurface later while reading.
	return
}

func mapFromEnviron(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, entry := range environ {
		parts := strings.SplitN(entry, "=", 2)
		out[parts[0]] = parts[1]
	}
	return out
}

func readWithMapInto(r io.Reader, env map[string]string, prefix string, config interface{}) error {
	var upstreamErr error
	upstreamErr = gcfg.ReadInto(config, r)
	if gcfg.FatalOnly(upstreamErr) != nil {
		return upstreamErr
	}
	if prefix != "" && !strings.HasSuffix(prefix, "_") {
		prefix = prefix + "_"
	}
	// We can assert that config is a pointer to a struct at this point.
	ref := reflect.ValueOf(config).Elem()
	err := setGcfgWithEnvMap(ref, prefix, env)
	if err == nil {
		return upstreamErr
	}
	return err
}

func fieldToEnvVar(field reflect.StructField) string {
	t := field.Tag.Get("gcfg")
	if t != "" {
		// we need to replace dashes with underscores for consistency
		// with field.Name, which uses this convention automatically
		return strings.ToUpper(strings.ReplaceAll(t, "-", "_"))
	}
	return strings.ToUpper(field.Name)
}

// flatField describes one settable property of a section, after flattening
// anonymous embedded structs.
type flatField struct {
	// name is the environment variable component for this property.
	name string
	// index is the field index path, for reflect.Value.FieldByIndex.
	index []int
	// typ is the property's type.
	typ reflect.Type
}

// flatFields returns the properties of a section struct, flattening anonymous
// embedded structs. gcfg addresses the fields of an embedded struct as though
// they had been declared on the outer struct -- an embedded struct is not
// addressable by its own type name, even when it implements
// encoding.TextUnmarshaler -- so environment variables have to do the same, or
// a property that the configuration file exposes normally would be silently
// unreachable.
//
// Shadowing follows Go's promotion rules: a field at a shallower depth hides a
// same-named field at a deeper one, and two same-named fields at the same depth
// are both unreachable.
func flatFields(t reflect.Type) []flatField {
	type candidate struct {
		field flatField
		depth int
		// count is the number of fields seen at depth, so that we can
		// drop ambiguous names.
		count int
	}
	byName := make(map[string]*candidate)
	var order []string

	var walk func(reflect.Type, []int, int)
	walk = func(st reflect.Type, prefix []int, depth int) {
		for i := 0; i < st.NumField(); i++ {
			sf := st.Field(i)
			index := append(append(make([]int, 0, len(prefix)+1), prefix...), i)
			// Embedding an unexported type gives the field an
			// unexported name, but its own exported fields are still
			// settable through it, and gcfg sets them, so recurse
			// before rejecting unexported fields.
			if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
				walk(sf.Type, index, depth+1)
				continue
			}
			if !sf.IsExported() {
				continue
			}
			name := fieldToEnvVar(sf)
			found, ok := byName[name]
			if !ok {
				byName[name] = &candidate{
					field: flatField{name: name, index: index, typ: sf.Type},
					depth: depth,
					count: 1,
				}
				order = append(order, name)
				continue
			}
			switch {
			case depth < found.depth:
				found.field = flatField{name: name, index: index, typ: sf.Type}
				found.depth = depth
				found.count = 1
			case depth == found.depth:
				found.count++
			}
		}
	}
	walk(t, nil, 0)

	out := make([]flatField, 0, len(order))
	for _, name := range order {
		if c := byName[name]; c.count == 1 {
			out = append(out, c.field)
		}
	}
	return out
}

// setField applies an environment variable's value to a single field. Slices
// append, matching gcfg's handling of a variable repeated in a file.
func setField(f reflect.Value, val string) error {
	newRef, err := valFromEnvVar(f.Type(), val)
	if err != nil {
		return err
	}
	if f.Kind() == reflect.Slice {
		f.Set(reflect.AppendSlice(f, newRef))
	} else {
		f.Set(newRef)
	}
	return nil
}

func setGcfgWithEnvMap(ref reflect.Value, prefix string, env map[string]string) error {
	refType := ref.Type()
	for i := 0; i < refType.NumField(); i++ {
		sec := ref.Field(i)
		secStructField := refType.Field(i)
		secType := sec.Type()
		secPrefix := prefix + fieldToEnvVar(secStructField)

		if !sec.CanSet() || !secStructField.IsExported() {
			continue
		}

		// Sections can be either structs or map[string]*struct.
		if sec.Kind() == reflect.Struct {
			for _, ff := range flatFields(secType) {
				f := sec.FieldByIndex(ff.index)
				if !f.CanSet() {
					continue
				}
				val, found := env[secPrefix+"_"+ff.name]
				if !found {
					continue
				}
				if err := setField(f, val); err != nil {
					return err
				}
			}
			continue
		}
		if sec.Kind() == reflect.Map {
			subsecType := secType.Elem().Elem()
			// We don't know in advance what the subsections might
			// be named -- or if they will be present in the
			// existing map.
			matchingEnv := make(map[string]string)
			for e := range env {
				if !strings.HasPrefix(e, secPrefix+"_") {
					continue
				}
				newKey := strings.Replace(e, secPrefix+"_", "", 1)
				if newKey == "" {
					continue
				}
				matchingEnv[newKey] = env[e]
			}

			// First, handle overrides for existing keys in the map.
			iter := sec.MapRange()
			for iter.Next() {
				key := iter.Key().Interface().(string) + "_"
				if key == "_" {
					key = ""
				}
				subsec := iter.Value().Elem()
				for _, ff := range flatFields(subsecType) {
					f := subsec.FieldByIndex(ff.index)
					envVar := key + ff.name
					if !f.CanSet() {
						continue
					}
					val, found := matchingEnv[envVar]
					if !found {
						continue
					}
					delete(matchingEnv, envVar)
					if err := setField(f, val); err != nil {
						return err
					}
				}
			}
			if len(matchingEnv) == 0 {
				continue
			}

			// Second, handle environment variables that will create
			// new subsections. We also need to account for when
			// there is a "default value" struct for these new
			// subsections.
			defaults := ref.FieldByName(
				"Default_" + secStructField.Name)
			if defaults == (reflect.Value{}) {
				defaults = reflect.Zero(subsecType)
			}
			for _, ff := range flatFields(subsecType) {
				suf := "_" + ff.name
				for e, v := range matchingEnv {
					if !strings.HasSuffix(e, suf) {
						continue
					}
					k := strings.Replace(e, suf, "", 1)
					key := reflect.ValueOf(k)
					if sec.IsNil() {
						m := reflect.MakeMap(sec.Type())
						sec.Set(m)
					}
					f := sec.MapIndex(key)
					if f == (reflect.Value{}) {
						f = reflect.New(subsecType)
						f.Elem().Set(defaults)
						sec.SetMapIndex(key, f)
					}
					if err := setField(f.Elem().FieldByIndex(ff.index), v); err != nil {
						return err
					}
					// TODO: Does this have any unfortunate
					// side-effects?
					delete(matchingEnv, e)
				}
			}

			continue
		}

		// Non-section fields do not cause gcfg to error, so we can
		// ignore them here as well.
	}
	return nil
}

func valFromEnvVar(t reflect.Type, env string) (reflect.Value, error) {
	kind := t.Kind()

	// Try encoding.TextUnmarshaler first. We need to handle both values
	// that may have a method with a pointer receiver as well as pointers
	// themselves.
	if t.Kind() == reflect.Ptr {
		// In this case we replace the existing pointer with a new one.
		ptr := reflect.New(t.Elem())
		unmarshaller, ok := ptr.Interface().(encoding.TextUnmarshaler)
		if ok {
			// Slice types have to be unmarshalled per entry.
			if ptr.Elem().Kind() == reflect.Slice {
				parts := strings.Split(env, ",")
				for i := range parts {
					err := unmarshaller.UnmarshalText([]byte(parts[i]))
					// Stop unmarshalling and return on an error.
					if err != nil {
						return ptr, err
					}
				}
			} else {
				// Otherwise just unmarshal the env var directly.
				return ptr, unmarshaller.UnmarshalText([]byte(env))
			}
		}
	} else {
		ptr := reflect.New(t)
		unmarshaller, ok := ptr.Interface().(encoding.TextUnmarshaler)
		if ok {
			// Slice types have to be unmarshalled per entry.
			if t.Kind() == reflect.Slice {
				parts := strings.Split(env, ",")
				for i := range parts {
					err := unmarshaller.UnmarshalText([]byte(parts[i]))
					// Stop unmarshalling and return on an error.
					if err != nil {
						return ptr.Elem(), err
					}
				}
			} else {
				// Otherwise just unmarshal the env var directly.
				return ptr.Elem(), unmarshaller.UnmarshalText([]byte(env))
			}
		}
	}

	// A defined type such as `type Format string` has a basic kind, but a
	// value of its underlying type is not assignable to it, so every scalar
	// result below is converted to t before it is returned.
	switch t.Kind() {
	case reflect.Ptr:
		ref, err := valFromEnvVar(t.Elem(), env)
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(ref)
		return ptr, err
	case reflect.String:
		return reflect.ValueOf(env).Convert(t), nil
	case reflect.Bool:
		// gcfg's boolean parser does not strip whitespace on its own.
		env = strings.ReplaceAll(env, " ", "")
		b, err := types.ParseBool(env)
		return reflect.ValueOf(b).Convert(t), err
	case reflect.Int:
		var i int
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Int8:
		var i int8
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Int16:
		var i int16
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Int32:
		var i int32
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Int64:
		var i int64
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Uint:
		var i uint
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Uint8:
		var i uint8
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Uint16:
		var i uint16
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Uint32:
		var i uint32
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Uint64:
		var i uint64
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i).Convert(t), err
	case reflect.Float32:
		var f float32
		err := types.ScanFully(&f, env, 'v')
		return reflect.ValueOf(f).Convert(t), err
	case reflect.Float64:
		var f float64
		err := types.ScanFully(&f, env, 'v')
		return reflect.ValueOf(f).Convert(t), err
	case reflect.Slice:
		parts := strings.Split(env, ",")
		out := reflect.MakeSlice(t, len(parts), len(parts))
		for i := range parts {
			elt, err := valFromEnvVar(t.Elem(), parts[i])
			if err != nil {
				return reflect.Zero(t), err
			}
			out.Index(i).Set(elt)
		}
		return out, nil
	default:
		return reflect.Zero(t), fmt.Errorf("unsupported type: %s", kind)
	}
}
