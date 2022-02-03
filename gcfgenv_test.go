// Copyright 2022 RStudio, PBC
// SPDX-License-Identifier: Apache-2.0

package gcfgenv

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/check.v1"
	"gopkg.in/gcfg.v1"
)

type Suite struct{}

// Type alias to test encoding.TextUnmarshaler behaviour.
type lowerString string

func (w *lowerString) UnmarshalText(text []byte) error {
	*w = lowerString(strings.ToLower(string(text)))
	return nil
}

var lowerStringValue = lowerString("value")

var conversionCases = []struct {
	t        reflect.Type
	env      string
	want     reflect.Value
	errMatch string
}{
	// Strings.
	{reflect.TypeOf(""), "value", reflect.ValueOf("value"), ""},
	// Various ways to express a boolean.
	{reflect.TypeOf(false), "0", reflect.ValueOf(false), ""},
	{reflect.TypeOf(false), "1", reflect.ValueOf(true), ""},
	{reflect.TypeOf(false), "YES", reflect.ValueOf(true), ""},
	{reflect.TypeOf(false), "no", reflect.ValueOf(false), ""},
	{reflect.TypeOf(false), "on", reflect.ValueOf(true), ""},
	{reflect.TypeOf(false), "OFF", reflect.ValueOf(false), ""},
	// Numbers.
	{reflect.TypeOf(int(0)), "100", reflect.ValueOf(int(100)), ""},
	{reflect.TypeOf(int8(0)), "100", reflect.ValueOf(int8(100)), ""},
	{reflect.TypeOf(int16(0)), "100", reflect.ValueOf(int16(100)), ""},
	{reflect.TypeOf(int32(0)), "100", reflect.ValueOf(int32(100)), ""},
	{reflect.TypeOf(int64(0)), "0xea61f", reflect.ValueOf(int64(0xea61f)), ""},
	{reflect.TypeOf(uint(0)), "100", reflect.ValueOf(uint(100)), ""},
	{reflect.TypeOf(uint8(0)), "0xff", reflect.ValueOf(uint8(255)), ""},
	{reflect.TypeOf(uint16(0)), "100", reflect.ValueOf(uint16(100)), ""},
	{reflect.TypeOf(uint32(0)), "100", reflect.ValueOf(uint32(100)), ""},
	{reflect.TypeOf(uint64(0)), "100", reflect.ValueOf(uint64(100)), ""},
	{reflect.TypeOf(float32(0)), "3.14159", reflect.ValueOf(float32(3.14159)), ""},
	{reflect.TypeOf(float64(0)), "2.4e-19", reflect.ValueOf(float64(2.4e-19)), ""},
	// Pointers.
	{reflect.TypeOf(new(string)), "value", reflect.ValueOf("value"), ""},
	{reflect.TypeOf(new(bool)), "on", reflect.ValueOf(true), ""},
	{reflect.TypeOf(new(int)), "100", reflect.ValueOf(int(100)), ""},
	{reflect.TypeOf(new(float32)), "3.14159", reflect.ValueOf(float32(3.14159)), ""},
	// Slices.
	{reflect.TypeOf([]string{}), "v1", reflect.ValueOf([]string{"v1"}), ""},
	{reflect.TypeOf([]string{}), "v1,v2,v3", reflect.ValueOf([]string{"v1", "v2", "v3"}), ""},
	{reflect.TypeOf([]int8{}), "34,0x1a", reflect.ValueOf([]int8{34, 0x1a}), ""},
	// TextUnmarshaler.
	{reflect.TypeOf(lowerStringValue), "VALUE", reflect.ValueOf(lowerStringValue), ""},
	{reflect.TypeOf(new(lowerString)), "VALUE", reflect.ValueOf(lowerStringValue), ""},
	// Whitespace is ignored.
	{reflect.TypeOf(false), "  no    ", reflect.ValueOf(false), ""},
	{reflect.TypeOf(int(0)), "  0xff    ", reflect.ValueOf(int(0xff)), ""},
	{reflect.TypeOf(int(0)), "  106    ", reflect.ValueOf(int(106)), ""},
	{reflect.TypeOf(float64(0)), " 2.4e-19    ", reflect.ValueOf(float64(2.4e-19)), ""},
	// Overflow.
	{reflect.TypeOf(int8(0)), "128", zeroOf(int8(0)), ".*integer overflow.*"},
	{reflect.TypeOf(uint8(0)), "256", zeroOf(uint8(0)), ".*integer overflow.*"},
	// Parse failures.
	{reflect.TypeOf(false), "notabool", zeroOf(false), "failed to parse.*"},
	{reflect.TypeOf(int8(0)), "notanint", zeroOf(int8(0)), ".*failed to parse.*"},
	{reflect.TypeOf(float32(0)), "notafloat", zeroOf(float32(0)), ".*failed to parse.*"},
	{reflect.TypeOf(new(bool)), "notabool", zeroOf(false), "failed to parse.*"},
	// Unsupported types.
	{reflect.TypeOf(new(chan int)), "", zeroOf(make(chan int)), "unsupported type.*"},
	{reflect.TypeOf([][3]int{}), "", zeroOf([][3]int{}), "unsupported type.*"},
}

func zeroOf(i interface{}) reflect.Value {
	return reflect.Zero(reflect.TypeOf(i))
}

func (s *Suite) TestConversion(c *check.C) {
	for i, tc := range conversionCases {
		got, err := valFromEnvVar(tc.t, tc.env)
		if got.Kind() == reflect.Ptr && !got.IsNil() {
			// Pointers won't have the same address, so we compare
			// by the values they point to instead.
			got = got.Elem()
		}
		c.Check(got.Interface(), check.DeepEquals, tc.want.Interface(),
			check.Commentf("test case %d", i))
		if tc.errMatch == "" {
			c.Check(err, check.Equals, nil,
				check.Commentf("test case %d", i))
		} else {
			c.Check(err, check.ErrorMatches, tc.errMatch,
				check.Commentf("test case %d", i))
		}
	}
}

func (s *Suite) TestFileSupport(c *check.C) {
	type sec struct {
		Field string
	}
	type config struct {
		Sec sec
	}
	var err error
	var cfg config
	f, _ := os.CreateTemp(os.TempDir(), ".cfg")
	defer f.Close()

	err = ReadFileWithEnvInto("doesnotexist.cfg", "", &cfg)
	c.Check(err, check.ErrorMatches, ".*no such file or directory")

	// Empty file.
	err = ReadFileWithEnvInto(f.Name(), "", &cfg)
	c.Check(err, check.IsNil)

	// File without BOM.
	f.WriteString("[sec]\nfield = value")
	err = ReadFileWithEnvInto(f.Name(), "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: sec{"value"}})

	// File with BOM.
	f.Seek(0, 0)
	f.Write(utf8BOM)
	f.WriteString("[sec]\nfield = value")
	err = ReadFileWithEnvInto(f.Name(), "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: sec{"value"}})
}

func (s *Suite) TestMapFromEnviron(c *check.C) {
	environ := []string{
		"APPNAME_SEC_FIELD=geese",
		"APPNAME_SEC_k1_FIELD=cats",
		"APPNAME_SEC_k1_OTHER_FIELD=zebras,elephants",
		// Something base64-encoded, which will have a trailing '='.
		"APPNAME_SEC_k1_KEY=f4Q8N6PFcZKi9EK8NfvRbDgeUMkHyw9mXkMK/kPEi5Q=",
	}
	expected := map[string]string{
		"APPNAME_SEC_FIELD":          "geese",
		"APPNAME_SEC_k1_FIELD":       "cats",
		"APPNAME_SEC_k1_OTHER_FIELD": "zebras,elephants",
		"APPNAME_SEC_k1_KEY":         "f4Q8N6PFcZKi9EK8NfvRbDgeUMkHyw9mXkMK/kPEi5Q=",
	}
	c.Check(mapFromEnviron(environ), check.DeepEquals, expected)
}

func (s *Suite) TestUpstreamErrors(c *check.C) {
	type sec struct {
		Field string
	}
	type config struct {
		Sec sec
	}
	var err error
	var cfg config
	var r *strings.Reader
	configEnvVars := map[string]string{
		"SEC_FIELD": "set",
	}

	// Parser errors should be surfaced immediately without proceeding to
	// overrides.
	r = strings.NewReader("[sec1]\nfi eld = value")
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.Not(check.IsNil))
	c.Check(cfg, check.DeepEquals, config{})

	// Parser warnings (e.g. extra data) should be surfaced after
	// proceeding, so that users can still wrap the call in
	// gcfg.FatalOnly().
	r = strings.NewReader("[sec1]\nother = value")
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.Not(check.IsNil))
	c.Check(cfg, check.DeepEquals, config{Sec: sec{"set"}})
}

func (s *Suite) TestEnvAndPrefixes(c *check.C) {
	type sec struct {
		Field string
	}
	type config struct {
		Sec1 sec
		Sec2 sec
	}
	var err error
	var cfg config
	var configFilledWithEnvVars config
	var r *strings.Reader

	os.Setenv("APPNAME_SEC1_FIELD", "set")
	os.Setenv("SEC2_FIELD", "set")
	defer func() {
		os.Unsetenv("APPNAME_SEC1_FIELD")
		os.Unsetenv("SEC2_FIELD")
	}()

	cfg = config{}
	configFilledWithEnvVars = config{Sec2: sec{"set"}}
	r = strings.NewReader("")
	err = ReadWithEnvInto(r, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)

	cfg = config{}
	configFilledWithEnvVars = config{Sec1: sec{"set"}}
	r = strings.NewReader("")
	err = ReadWithEnvInto(r, "APPNAME", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)

	cfg = config{}
	configFilledWithEnvVars = config{Sec1: sec{"set"}}
	r = strings.NewReader("")
	err = ReadWithEnvInto(r, "APPNAME_", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)
}

func (s *Suite) TestSections(c *check.C) {
	type sec1 struct {
		F1 string
		F2 int
	}

	type config struct {
		Sec1 sec1
		Sec2 sec1
	}

	var err error
	configString := `[sec1]
f1 = value

[sec2]
f1 = value
`
	configFilled := config{
		Sec1: sec1{"value", 0},
		Sec2: sec1{"value", 0},
	}
	configEnvVars := map[string]string{
		"SEC2_F1": "set",
	}
	configFilledWithEnvVars := configFilled
	configFilledWithEnvVars.Sec2.F1 = "set"

	cfg := config{}
	err = gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilled)

	cfg = config{}
	r := strings.NewReader(configString)
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)

	configEnvVars["SEC2_F2"] = "notanumber"
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.ErrorMatches, "failed to parse.*")
}

func (s *Suite) TestSkipPrivate(c *check.C) {
	type sec1 struct {
		F1      string
		private string
	}

	type config struct {
		Sec1    sec1
		private int
	}

	var err error
	configString := `[sec1]
f1 = value
`
	configFilled := config{
		Sec1: sec1{F1: "value"},
	}
	configEnvVars := map[string]string{
		"SEC1_PRIVATE": "noset",
	}
	configFilledWithEnvVars := configFilled

	cfg := config{}
	err = gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilled)

	cfg = config{}
	r := strings.NewReader(configString)
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)
}

func (s *Suite) TestSubsections(c *check.C) {
	type sec struct {
		F1      string
		F2      string
		F3      int
		private string
	}

	type config struct {
		Sec1         map[string]*sec
		Default_Sec1 sec
		Sec2         map[string]*sec
	}

	var err error
	configString := `[sec1]
f1 = geese

[sec1 "k1"]
f1 = cats

[sec1 "k2"]
f2 = dogs
`
	configFilled := config{
		Sec1: map[string]*sec{
			"":   {F1: "geese", F2: "default"},
			"k1": {F1: "cats", F2: "default"},
			"k2": {F2: "dogs"},
		},
		Default_Sec1: sec{F2: "default"},
	}
	configEnvVars := map[string]string{
		"SEC1_F2":    "set",
		"SEC1_k1_F1": "set",
		"SEC1_k2_F2": "set",
	}
	configFilledWithEnvVars := config{
		Sec1: map[string]*sec{
			"":   {F1: "geese", F2: "set"},
			"k1": {F1: "set", F2: "default"},
			"k2": {F2: "set"},
		},
		Default_Sec1: sec{F2: "default"},
	}

	cfg := config{
		Default_Sec1: sec{F2: "default"},
	}
	err = gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilled)

	cfg = config{
		Default_Sec1: sec{F2: "default"},
	}
	r := strings.NewReader(configString)
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)

	// Not all branches can be tested with a single configuration &
	// environment variable map, so they have to be broken up.

	configEnvVars["SEC1"] = "notset"
	configEnvVars["SEC1_"] = "notset"
	configEnvVars["SEC1_k3_F1"] = "set"
	configEnvVars["SEC1_k3_F3"] = "1"
	configEnvVars["SEC2_k1_F1"] = "set"
	configFilledWithEnvVars.Sec1["k3"] = &sec{
		F1: "set", F2: "default", F3: 1,
	}
	configFilledWithEnvVars.Sec2 = map[string]*sec{
		"k1": &sec{F1: "set"},
	}
	cfg = config{
		Default_Sec1: sec{F2: "default"},
	}
	r = strings.NewReader(configString)
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)

	configEnvVars["SEC2_k1_F3"] = "notanumber"
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.ErrorMatches, "failed to parse.*")

	configEnvVars["SEC2_k1_F3"] = "1"
	configEnvVars["SEC2_k3_F3"] = "notanumber"
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.ErrorMatches, "failed to parse.*")
}

func (s *Suite) TestGcfgTags(c *check.C) {
	c.ExpectFailure("not yet implemented")

	type sec1 struct {
		F1 string `gcfg:"another-name"`
	}

	type config struct {
		Sec1 sec1 `gcfg:"sec2"`
	}

	var err error
	configString := `[sec2]
another-name = value
`
	configFilled := config{
		Sec1: sec1{F1: "value"},
	}
	configEnvVars := map[string]string{
		"SEC2_ANOTHER_NAME": "set",
	}
	configFilledWithEnvVars := config{
		Sec1: sec1{F1: "set"},
	}

	cfg := config{}
	err = gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilled)

	cfg = config{}
	r := strings.NewReader(configString)
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)
}

func Test(t *testing.T) {
	_ = check.Suite(&Suite{})
	check.TestingT(t)
}
