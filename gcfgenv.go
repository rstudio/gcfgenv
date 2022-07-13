// Copyright 2022 RStudio, PBC
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
			for j := 0; j < secType.NumField(); j++ {
				f := sec.Field(j)
				sf := secType.Field(j)
				envVar := secPrefix + "_" + fieldToEnvVar(sf)
				if !f.CanSet() || !sf.IsExported() {
					continue
				}
				val, found := env[envVar]
				if !found {
					continue
				}
				newRef, err := valFromEnvVar(f.Type(), val)
				if err != nil {
					return err
				}
				if f.Kind() == reflect.Slice {
					f.Set(reflect.AppendSlice(f, newRef))
				} else {
					f.Set(newRef)
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
				for j := 0; j < subsecType.NumField(); j++ {
					f := subsec.Field(j)
					sf := subsecType.Field(j)
					envVar := key + fieldToEnvVar(sf)
					if !f.CanSet() || !sf.IsExported() {
						continue
					}
					val, found := matchingEnv[envVar]
					if !found {
						continue
					}
					delete(matchingEnv, envVar)
					newRef, err := valFromEnvVar(f.Type(), val)
					if err != nil {
						return err
					}
					if f.Kind() == reflect.Slice {
						f.Set(reflect.AppendSlice(f.Elem(), newRef))
					} else {
						f.Set(newRef)
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
			for j := 0; j < subsecType.NumField(); j++ {
				sf := subsecType.Field(j)
				if !sf.IsExported() {
					continue
				}
				suf := "_" + fieldToEnvVar(sf)
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
					newRef, err := valFromEnvVar(sf.Type, v)
					if err != nil {
						return err
					}
					if f.Elem().Field(j).Kind() == reflect.Slice {
						f.Elem().Field(j).Set(reflect.AppendSlice(f.Elem().Field(j).Elem(), newRef))
					} else {
						f.Elem().Field(j).Set(newRef)
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
		ptr := reflect.New(t.Elem())
		unmarshaller, ok := ptr.Interface().(encoding.TextUnmarshaler)
		if ok {
			return ptr, unmarshaller.UnmarshalText([]byte(env))
		}
	} else {
		ptr := reflect.New(t)
		unmarshaller, ok := ptr.Interface().(encoding.TextUnmarshaler)
		if ok {
			return ptr.Elem(), unmarshaller.UnmarshalText([]byte(env))
		}
	}

	switch t.Kind() {
	case reflect.Ptr:
		ref, err := valFromEnvVar(t.Elem(), env)
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(ref)
		return ptr, err
	case reflect.String:
		return reflect.ValueOf(env), nil
	case reflect.Bool:
		// gcfg's boolean parser does not strip whitespace on its own.
		env = strings.ReplaceAll(env, " ", "")
		b, err := types.ParseBool(env)
		return reflect.ValueOf(b), err
	case reflect.Int:
		var i int
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Int8:
		var i int8
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Int16:
		var i int16
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Int32:
		var i int32
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Int64:
		var i int64
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Uint:
		var i uint
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Uint8:
		var i uint8
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Uint16:
		var i uint16
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Uint32:
		var i uint32
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Uint64:
		var i uint64
		err := types.ParseInt(&i, env, types.Dec|types.Hex)
		return reflect.ValueOf(i), err
	case reflect.Float32:
		var f float32
		err := types.ScanFully(&f, env, 'v')
		return reflect.ValueOf(f), err
	case reflect.Float64:
		var f float64
		err := types.ScanFully(&f, env, 'v')
		return reflect.ValueOf(f), err
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
