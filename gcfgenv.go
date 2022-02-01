package gcfgenv

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"gopkg.in/gcfg.v1"
	"gopkg.in/gcfg.v1/types"
)

func ReadWithEnvInto(r io.Reader, envPrefix string, config interface{}) error {
	env := mapFromEnviron(os.Environ())
	return readWithMapInto(r, env, envPrefix, config)
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
	err := gcfg.ReadInto(config, r)
	if err != nil {
		return err
	}
	if prefix != "" && !strings.HasSuffix(prefix, "_") {
		prefix = prefix + "_"
	}
	// We can assert that config is a pointer to a struct at this point.
	ref := reflect.ValueOf(config).Elem()
	return setGcfgWithEnvMap(ref, prefix, env)
}

func setGcfgWithEnvMap(ref reflect.Value, prefix string, env map[string]string) error {
	refType := ref.Type()
	for i := 0; i < refType.NumField(); i++ {
		sec := ref.Field(i)
		secStructField := refType.Field(i)
		secType := sec.Type()
		secPrefix := prefix + strings.ToUpper(secStructField.Name)

		if !sec.CanSet() || !secStructField.IsExported() {
			continue
		}

		// Sections can be either structs or map[string]*struct.
		if sec.Kind() == reflect.Struct {
			for j := 0; j < secType.NumField(); j++ {
				f := sec.Field(j)
				sf := secType.Field(j)
				envVar := secPrefix + "_" + strings.ToUpper(sf.Name)
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
				f.Set(newRef)
			}
			continue
		}
		if sec.Kind() == reflect.Map {
			// FIXME: Not yet implemented.
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
	default:
		return reflect.Zero(t), fmt.Errorf("unsupported type: %s", kind)
	}
}
