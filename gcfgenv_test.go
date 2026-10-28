// Copyright 2024 Posit Software, PBC
// SPDX-License-Identifier: Apache-2.0

package gcfgenv

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

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

// StringSliceType is a slice type to test encoding.TextUnmarshaler behaviour.
type StringSliceType []string

func (ss *StringSliceType) Length() int {
	return ss.Length()
}

func (ss *StringSliceType) Set(text string) error {
	*ss = append(*ss, text)
	return nil
}

func (ss *StringSliceType) UnmarshalText(text []byte) error {
	*ss = append(*ss, string(text))
	return nil
}

func (ss *StringSliceType) String() string {
	return strings.Join(*ss, "|")
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

// ReportCommon is exported so that it is addressable by its type name.
type ReportCommon struct {
	Flag bool
}

type reportSec struct {
	ReportCommon
	Field string
	Count int
}

type reportConfig struct {
	Plain         reportSec
	Keyed         map[string]*reportSec
	Default_Keyed reportSec
	Renamed       reportSec `gcfg:"other-name"`
}

func (s *Suite) TestReport(c *check.C) {
	env := map[string]string{
		// Applied.
		"APP_PLAIN_FIELD":        "x",
		"APP_PLAIN_FLAG":         "true",
		"APP_KEYED_k1_FIELD":     "x", // existing subsection
		"APP_KEYED_k2_FIELD":     "x", // new subsection
		"APP_KEYED_COUNT":        "1", // unnamed subsection
		"APP_OTHER_NAME_FIELD":   "x", // section renamed by a tag
		"APP_DEFAULT_KEYED_FLAG": "true",
		// Unused: each names a section but sets nothing.
		"APP_PLAIN_BOGUS":         "x",
		"APP_KEYED_k1_BOGUS":      "x",
		"APP_PLAIN_REPORTCOMMON":  "x", // embedded type that does not parse
		"APP_KEYED_REPORTCOMMON":  "x",
		"APP_DEFAULT_KEYED_BOGUS": "x",
		"APP_OTHER_NAME_BOGUS":    "x",
		// In neither list.
		"APP_OTHER_THING":   "x", // names no section
		"APP_PLAIN":         "x", // a section name alone
		"APP_RENAMED_FIELD": "x", // the Go name of a renamed section
		"PLAIN_FIELD":       "x", // no prefix
	}

	cfg := reportConfig{}
	report, err := readWithMapIntoReport(strings.NewReader("[keyed \"k1\"]\n"), env, "APP", &cfg)
	c.Assert(err, check.IsNil)
	c.Check(report.Applied, check.DeepEquals, []string{
		"APP_DEFAULT_KEYED_FLAG",
		"APP_KEYED_COUNT",
		"APP_KEYED_k1_FIELD",
		"APP_KEYED_k2_FIELD",
		"APP_OTHER_NAME_FIELD",
		"APP_PLAIN_FIELD",
		"APP_PLAIN_FLAG",
	})
	c.Check(report.Unused, check.DeepEquals, []string{
		"APP_DEFAULT_KEYED_BOGUS",
		"APP_KEYED_REPORTCOMMON",
		"APP_KEYED_k1_BOGUS",
		"APP_OTHER_NAME_BOGUS",
		"APP_PLAIN_BOGUS",
		"APP_PLAIN_REPORTCOMMON",
	})
	// The report reflects what was set.
	c.Check(cfg.Plain, check.DeepEquals, reportSec{ReportCommon: ReportCommon{Flag: true}, Field: "x"})
	c.Check(cfg.Keyed[""].Count, check.Equals, 1)
	c.Check(cfg.Renamed.Field, check.Equals, "x")
}

func (s *Suite) TestReportWithOverlappingSectionNames(c *check.C) {
	// A variable for section Sec_X also starts with Sec's prefix. Each is
	// reported once, against whichever section it sets.
	type sec struct {
		Field string
	}
	type config struct {
		Sec   sec
		Sec_X sec
	}

	cfg := config{}
	report, err := readWithMapIntoReport(strings.NewReader(""), map[string]string{
		"APP_SEC_FIELD":   "x",
		"APP_SEC_X_FIELD": "y",
		"APP_SEC_BOGUS":   "x",
		"APP_SEC_X_BOGUS": "x",
	}, "APP", &cfg)
	c.Assert(err, check.IsNil)
	c.Check(report, check.DeepEquals, Report{
		Applied: []string{"APP_SEC_FIELD", "APP_SEC_X_FIELD"},
		Unused:  []string{"APP_SEC_BOGUS", "APP_SEC_X_BOGUS"},
	})
	c.Check(cfg, check.DeepEquals, config{Sec: sec{Field: "x"}, Sec_X: sec{Field: "y"}})
}

// ReportSections is exported so that its sections are promoted.
type ReportSections struct {
	Inner reportSec
}

func (s *Suite) TestReportThroughAnEmbeddedStruct(c *check.C) {
	// A section promoted from an embedded struct is reported like any
	// other.
	type config struct {
		ReportSections
	}

	cfg := config{}
	report, err := readWithMapIntoReport(strings.NewReader(""), map[string]string{
		"APP_INNER_FIELD": "x",
		"APP_INNER_BOGUS": "x",
	}, "APP", &cfg)
	c.Assert(err, check.IsNil)
	c.Check(report, check.DeepEquals, Report{
		Applied: []string{"APP_INNER_FIELD"},
		Unused:  []string{"APP_INNER_BOGUS"},
	})
	c.Check(cfg.Inner.Field, check.Equals, "x")
}

func (s *Suite) TestReportSkipsPointerSections(c *check.C) {
	// A section declared as a pointer to a struct is not set from the
	// environment, so its variables are in neither list.
	type config struct {
		Ptr *reportSec
	}

	cfg := config{}
	report, err := readWithMapIntoReport(strings.NewReader(""), map[string]string{
		"APP_PTR_FIELD": "x",
		"APP_PTR_BOGUS": "x",
	}, "APP", &cfg)
	c.Assert(err, check.IsNil)
	c.Check(report, check.DeepEquals, Report{})
	c.Check(cfg.Ptr, check.IsNil)
}

func (s *Suite) TestReportWithoutAPrefix(c *check.C) {
	// With no prefix every variable could name a section, so nothing is
	// reported as unused.
	cfg := reportConfig{}
	report, err := readWithMapIntoReport(strings.NewReader(""), map[string]string{
		"PLAIN_FIELD": "x",
		"PLAIN_BOGUS": "x",
	}, "", &cfg)
	c.Assert(err, check.IsNil)
	c.Check(report, check.DeepEquals, Report{Applied: []string{"PLAIN_FIELD"}})
}

func (s *Suite) TestReportOnError(c *check.C) {
	cfg := reportConfig{}
	report, err := readWithMapIntoReport(strings.NewReader(""), map[string]string{
		"APP_PLAIN_FIELD": "x",
		"APP_PLAIN_COUNT": "notanumber",
	}, "APP", &cfg)
	c.Check(err, check.ErrorMatches, "environment variable APP_PLAIN_COUNT: .*")
	c.Check(report, check.DeepEquals, Report{})

	report, err = readWithMapIntoReport(strings.NewReader("[plain\n"), map[string]string{
		"APP_PLAIN_FIELD": "x",
	}, "APP", &cfg)
	c.Check(err, check.NotNil)
	c.Check(report, check.DeepEquals, Report{})
}

func (s *Suite) TestReportWithUpstreamWarnings(c *check.C) {
	// gcfg's non-fatal warnings do not stop the read, so the report is
	// still complete.
	cfg := reportConfig{}
	report, err := readWithMapIntoReport(strings.NewReader("[plain]\nunknown = x\n"), map[string]string{
		"APP_PLAIN_FIELD": "x",
		"APP_PLAIN_BOGUS": "x",
	}, "APP", &cfg)
	c.Check(err, check.NotNil)
	c.Check(gcfg.FatalOnly(err), check.IsNil)
	c.Check(report, check.DeepEquals, Report{
		Applied: []string{"APP_PLAIN_FIELD"},
		Unused:  []string{"APP_PLAIN_BOGUS"},
	})
}

func (s *Suite) TestReportFromTheProcessEnvironment(c *check.C) {
	os.Setenv("GCFGENVTEST_PLAIN_FIELD", "x")
	os.Setenv("GCFGENVTEST_PLAIN_BOGUS", "x")
	defer func() {
		os.Unsetenv("GCFGENVTEST_PLAIN_FIELD")
		os.Unsetenv("GCFGENVTEST_PLAIN_BOGUS")
	}()
	want := Report{
		Applied: []string{"GCFGENVTEST_PLAIN_FIELD"},
		Unused:  []string{"GCFGENVTEST_PLAIN_BOGUS"},
	}

	cfg := reportConfig{}
	report, err := ReadWithEnvIntoReport(strings.NewReader(""), "GCFGENVTEST", &cfg)
	c.Check(err, check.IsNil)
	c.Check(report, check.DeepEquals, want)

	f, err := os.CreateTemp(c.MkDir(), "*.cfg")
	c.Assert(err, check.IsNil)
	f.WriteString("[plain]\ncount = 2\n")
	f.Close()
	cfg = reportConfig{}
	report, err = ReadFileWithEnvIntoReport(f.Name(), "GCFGENVTEST", &cfg)
	c.Check(err, check.IsNil)
	c.Check(report, check.DeepEquals, want)
	c.Check(cfg.Plain.Count, check.Equals, 2)

	report, err = ReadFileWithEnvIntoReport("doesnotexist.cfg", "GCFGENVTEST", &cfg)
	c.Check(err, check.NotNil)
	c.Check(report, check.DeepEquals, Report{})
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
	c.Check(err, check.ErrorMatches, "environment variable SEC2_F2: failed to parse.*")
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

[sec1 "env|overwrite- Rich"]
f3 = 4

[sec1 "env|overwrite-_Rich"]
f3 = 5

[sec1 "some very blank space"]
f1 = three
`
	configFilled := config{
		Sec1: map[string]*sec{
			"":                      {F1: "geese", F2: "default"},
			"k1":                    {F1: "cats", F2: "default"},
			"k2":                    {F2: "dogs"},
			"env|overwrite- Rich":   {F3: 4, F2: "default"},
			"env|overwrite-_Rich":   {F3: 5, F2: "default"},
			"some very blank space": {F1: "three", F2: "default"},
		},
		Default_Sec1: sec{F2: "default"},
	}
	configEnvVars := map[string]string{
		"SEC1_F2":                       "set",
		"SEC1_k1_F1":                    "set",
		"SEC1_k2_F2":                    "set",
		"SEC1_env|overwrite- Rich_F3":   "5",
		"SEC1_env|overwrite-_Rich_F1":   "hello",
		"SEC1_created- Rich_F1":         "another",
		"SEC1_some very blank space_F1": "four",
		// we do not parse the underscored variant as "whitespace" because
		// of non-deterministic behavior it can cause because maps are unordered
		"SEC1_some_very_blank_space_F1": "forty",
	}
	configFilledWithEnvVars := config{
		Sec1: map[string]*sec{
			"":                      {F1: "geese", F2: "set"},
			"k1":                    {F1: "set", F2: "default"},
			"k2":                    {F2: "set"},
			"env|overwrite- Rich":   {F3: 5, F2: "default"},
			"env|overwrite-_Rich":   {F1: "hello", F2: "default", F3: 5},
			"created- Rich":         {F1: "another", F2: "default"},
			"some very blank space": {F1: "four", F2: "default"},
			"some_very_blank_space": {F1: "forty", F2: "default"},
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
	c.Check(err, check.ErrorMatches, "environment variable SEC2_k1_F3: failed to parse.*")

	configEnvVars["SEC2_k1_F3"] = "1"
	configEnvVars["SEC2_k3_F3"] = "notanumber"
	err = readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.ErrorMatches, "environment variable SEC2_k3_F3: failed to parse.*")
}

func (s *Suite) TestErrorsNameTheVariable(c *check.C) {
	// A value that does not parse fails the read, as it does in a file. The
	// error names the variable, including the prefix, so that it can be
	// found, and wraps the parse error unchanged.
	type sec struct {
		Count int
	}
	type config struct {
		Plain sec
		Keyed map[string]*sec
	}

	_, parseErr := valFromEnvVar(reflect.TypeOf(0), "notanumber")
	c.Assert(parseErr, check.NotNil)

	for _, name := range []string{"APP_PLAIN_COUNT", "APP_KEYED_k1_COUNT", "APP_KEYED_k2_COUNT"} {
		cfg := config{}
		err := readWithMapInto(strings.NewReader("[keyed \"k1\"]\n"),
			map[string]string{name: "notanumber"}, "APP", &cfg)
		c.Check(err, check.ErrorMatches, "environment variable "+name+": failed to parse.*")
		c.Assert(errors.Unwrap(err), check.NotNil, check.Commentf("%s", name))
		c.Check(errors.Unwrap(err).Error(), check.Equals, parseErr.Error(), check.Commentf("%s", name))
	}
}

func (s *Suite) TestUnnamedSubsection(c *check.C) {
	// A variable with no subsection name sets the unnamed subsection, as a
	// plain [sec] heading does in a file, taking the same defaults.
	type sec struct {
		Field string
		Other string
	}
	type config struct {
		Sec         map[string]*sec
		Default_Sec sec
	}

	byFile := config{Default_Sec: sec{Other: "default"}}
	err := gcfg.ReadStringInto(&byFile, "[sec]\nfield = x\n")
	c.Check(err, check.IsNil)

	byEnv := config{Default_Sec: sec{Other: "default"}}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_FIELD": "x",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Check(byEnv, check.DeepEquals, byFile)
	c.Check(byEnv.Sec, check.DeepEquals, map[string]*sec{"": {Field: "x", Other: "default"}})

	// Alongside a named subsection.
	byEnv = config{Default_Sec: sec{Other: "default"}}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_FIELD":    "x",
		"SEC_k1_FIELD": "y",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Check(byEnv.Sec, check.DeepEquals, map[string]*sec{
		"":   {Field: "x", Other: "default"},
		"k1": {Field: "y", Other: "default"},
	})

	// An unnamed subsection that already exists is filled in place.
	byEnv = config{Sec: map[string]*sec{"": {Other: "seeded"}}}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_FIELD": "x",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Check(byEnv.Sec, check.DeepEquals, map[string]*sec{"": {Field: "x", Other: "seeded"}})
}

func (s *Suite) TestUnnamedSubsectionDoesNotOverrideAKey(c *check.C) {
	// SEC_A_B could be property A_B of the unnamed subsection or property B
	// of subsection A. The keyed reading is the one older versions applied,
	// so it wins.
	type sec struct {
		A_B string
		B   string
	}
	type config struct {
		Sec map[string]*sec
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), map[string]string{"SEC_A_B": "x"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec, check.DeepEquals, map[string]*sec{"A": {B: "x"}})
}

func (s *Suite) TestOverlappingPropertyNamesFollowDeclarationOrder(c *check.C) {
	// SEC_K_OTHER_FIELD could be property Other_Field of subsection K or
	// property Field of subsection K_OTHER. The property declared first
	// claims it.
	type fieldFirst struct {
		Field       string
		Other_Field string
	}
	type otherFirst struct {
		Other_Field string
		Field       string
	}
	type config struct {
		FieldFirst map[string]*fieldFirst
		OtherFirst map[string]*otherFirst
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), map[string]string{
		"FIELDFIRST_K_OTHER_FIELD": "x",
		"OTHERFIRST_K_OTHER_FIELD": "x",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.FieldFirst, check.DeepEquals, map[string]*fieldFirst{"K_OTHER": {Field: "x"}})
	c.Check(cfg.OtherFirst, check.DeepEquals, map[string]*otherFirst{"K": {Other_Field: "x"}})
}

func (s *Suite) TestGcfgTags(c *check.C) {
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

func (s *Suite) TestSliceEnvVars(c *check.C) {
	type sec struct {
		Field []string
	}
	type secAlt struct {
		Custom StringSliceType
	}
	type config struct {
		Sec1 sec
		Sec2 sec
		Sec3 sec
		Sec4 secAlt
	}

	var err error
	var cfg config
	var configFilledWithEnvVars config
	var r *strings.Reader

	os.Setenv("APPNAME_SEC1_FIELD", "one,two,three")
	os.Setenv("APPNAME_SEC2_FIELD", "a,b,c")
	os.Setenv("APPNAME_SEC3_FIELD", "1,2,3")
	os.Setenv("APPNAME_SEC4_CUSTOM", "x,y")
	defer func() {
		os.Unsetenv("APPNAME_SEC1_FIELD")
		os.Unsetenv("APPNAME_SEC2_FIELD")
		os.Unsetenv("APPNAME_SEC3_FIELD")
		os.Unsetenv("APPNAME_SEC4_CUSTOM")
	}()

	cfg = config{
		// set a default
		Sec2: sec{
			[]string{"q", "r", "s"},
		},
	}
	configFilledWithEnvVars = config{
		Sec1: sec{[]string{"hi", "one", "two", "three"}},
		Sec2: sec{[]string{"q", "r", "s", "a", "b", "c"}},
		Sec3: sec{[]string{"1", "2", "3"}},
		Sec4: secAlt{StringSliceType{"x", "y"}},
	}
	r = strings.NewReader("[Sec1]\nField=hi")
	err = ReadWithEnvInto(r, "APPNAME", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)
}

func (s *Suite) TestSlicePointerEnvVars(c *check.C) {
	type sec struct {
		Custom *StringSliceType
	}

	type config struct {
		Sec sec
	}

	var ptr = new(StringSliceType)
	var cfg config
	var r *strings.Reader
	var err error

	os.Setenv("APPNAME_SEC_CUSTOM", "x,y,z")
	defer func() {
		os.Unsetenv("APPNAME_SEC_CUSTOM")
	}()

	cfg = config{
		Sec: sec{ptr},
	}

	err = ptr.UnmarshalText([]byte("a"))
	c.Check(err, check.IsNil)
	err = ptr.UnmarshalText([]byte("b"))
	c.Check(err, check.IsNil)

	r = strings.NewReader("")
	err = ReadWithEnvInto(r, "APPNAME", &cfg)
	c.Check(err, check.IsNil)

	// It is noteworthy in this case that a,b are dropped
	// because we replace the existing pointer with a new one.
	c.Check(*cfg.Sec.Custom, check.HasLen, 3)
	c.Check(cfg.Sec.Custom.String(), check.Equals, "x|y|z")
	c.Check(*cfg.Sec.Custom, check.DeepEquals, StringSliceType{"x", "y", "z"})
}

// base is embedded to check that the fields of an embedded struct are
// addressable by environment variable, as they are in a configuration file.
type base struct {
	Issuer string
	Scope  []string
	Nested int
}

type outerBase struct {
	base
	Audience string
}

// definedString and friends exercise defined types whose underlying type is a
// basic kind. A value of the underlying type is not assignable to them.
type definedString string

type definedInt int

type definedBool bool

func (s *Suite) TestEmbeddedStructs(c *check.C) {
	type sec struct {
		base
		Audience string
	}
	type config struct {
		Sec sec
	}

	configString := `[sec]
issuer = fromfile
audience = fromfile
`
	// gcfg addresses an embedded struct's fields as though they were
	// declared on the outer struct, so environment variables must too.
	configEnvVars := map[string]string{
		"SEC_ISSUER":   "set",
		"SEC_NESTED":   "7",
		"SEC_SCOPE":    "a,b",
		"SEC_AUDIENCE": "set",
		// The embedded struct is not addressable by its type name,
		// matching gcfg, which reports that it cannot store data there.
		"SEC_BASE": "notset",
	}

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: sec{
		base:     base{Issuer: "fromfile"},
		Audience: "fromfile",
	}})

	cfg = config{}
	err = readWithMapInto(strings.NewReader(configString), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: sec{
		base:     base{Issuer: "set", Scope: []string{"a", "b"}, Nested: 7},
		Audience: "set",
	}})
}

func (s *Suite) TestEmbeddedStructsNested(c *check.C) {
	// An embedded struct within an embedded struct is flattened all the way
	// up.
	type sec struct {
		outerBase
		Extra string
	}
	type config struct {
		Sec sec
	}

	configEnvVars := map[string]string{
		"SEC_ISSUER":   "set",
		"SEC_AUDIENCE": "set",
		"SEC_EXTRA":    "set",
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: sec{
		outerBase: outerBase{base: base{Issuer: "set"}, Audience: "set"},
		Extra:     "set",
	}})
}

func (s *Suite) TestEmbeddedStructsShadowed(c *check.C) {
	// A field declared on the outer struct hides the embedded one, as in Go
	// and as in gcfg.
	type sec struct {
		base
		Issuer string
	}
	type config struct {
		Sec sec
	}

	configEnvVars := map[string]string{"SEC_ISSUER": "set"}

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, "[sec]\nissuer = fromfile\n")
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Issuer, check.Equals, "fromfile")
	c.Check(cfg.Sec.base.Issuer, check.Equals, "")

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Issuer, check.Equals, "set")
	c.Check(cfg.Sec.base.Issuer, check.Equals, "")
}

func (s *Suite) TestEmbeddedStructsAmbiguous(c *check.C) {
	type otherBase struct {
		Issuer string
	}
	// Two embedded structs at the same depth both declaring Issuer: the name
	// is unreachable, as it is in Go.
	type sec struct {
		base
		otherBase
	}
	type config struct {
		Sec sec
	}

	configEnvVars := map[string]string{"SEC_ISSUER": "notset"}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.base.Issuer, check.Equals, "")
	c.Check(cfg.Sec.otherBase.Issuer, check.Equals, "")
}

func (s *Suite) TestEmbeddedSubsections(c *check.C) {
	type sec struct {
		base
		Audience string
	}
	type config struct {
		Sec map[string]*sec
	}

	// k1 already exists in the file; k2 is created from the environment
	// alone. Both branches have to flatten the embedded struct.
	configString := `[sec "k1"]
audience = fromfile
`
	configEnvVars := map[string]string{
		"SEC_k1_ISSUER": "set",
		"SEC_k1_SCOPE":  "a,b",
		"SEC_k2_ISSUER": "set",
		"SEC_k2_SCOPE":  "c",
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(configString), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: map[string]*sec{
		"k1": {
			base:     base{Issuer: "set", Scope: []string{"a", "b"}},
			Audience: "fromfile",
		},
		"k2": {base: base{Issuer: "set", Scope: []string{"c"}}},
	}})
}

func (s *Suite) TestSubsectionSlices(c *check.C) {
	// A slice property of a subsection, in both map branches. Setting one
	// used to panic in reflect.
	type sec struct {
		Field []string
	}
	type config struct {
		Sec map[string]*sec
	}

	configString := `[sec "k1"]
field = fromfile
`
	configEnvVars := map[string]string{
		"SEC_k1_FIELD": "a,b",
		"SEC_k2_FIELD": "c,d",
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(configString), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Sec: map[string]*sec{
		// As elsewhere, environment values append to the file's.
		"k1": {Field: []string{"fromfile", "a", "b"}},
		"k2": {Field: []string{"c", "d"}},
	}})
}

func (s *Suite) TestDefinedTypes(c *check.C) {
	// Defined types with a basic underlying kind used to panic in
	// reflect.Set, because the parsed value had the underlying type.
	type sec struct {
		Str  definedString
		Num  definedInt
		Flag definedBool
		List []definedString
	}
	type config struct {
		Sec map[string]*sec
		Alt sec
	}

	configEnvVars := map[string]string{
		"ALT_STR":     "set",
		"ALT_NUM":     "7",
		"ALT_FLAG":    "true",
		"ALT_LIST":    "a,b",
		"SEC_k1_STR":  "set",
		"SEC_k1_LIST": "c",
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Alt, check.DeepEquals, sec{
		Str:  "set",
		Num:  7,
		Flag: true,
		List: []definedString{"a", "b"},
	})
	c.Check(cfg.Sec["k1"], check.DeepEquals, &sec{
		Str:  "set",
		List: []definedString{"c"},
	})
}

func (s *Suite) TestEmbeddedStructByTypeName(c *check.C) {
	// An exported embedded struct is addressable by its type name as well as
	// through its promoted fields, which is what makes an embedded time.Time
	// usable. gcfg accepts both, so we do too.
	type sec struct {
		time.Time
		Other string
	}
	type config struct {
		Sec sec
	}

	stamp := "2020-01-02T03:04:05Z"
	want, err := time.Parse(time.RFC3339, stamp)
	c.Assert(err, check.IsNil)

	cfg := config{}
	err = gcfg.ReadStringInto(&cfg, "[sec]\ntime = "+stamp+"\n")
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Time.Equal(want), check.Equals, true)

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{"SEC_TIME": stamp}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Time.Equal(want), check.Equals, true)
}

func (s *Suite) TestEmbeddedStructByTypeNameNeedsAParseableType(c *check.C) {
	// An embedded struct whose type cannot be parsed from text is not
	// settable by its type name. gcfg accepts the name in a file and then
	// fails to parse the value; the environment variable is left unused
	// instead. Its promoted fields are still reachable.
	type Common struct {
		Flag bool
	}
	type sec struct {
		Common
		Other string
	}
	type config struct {
		Plain sec
		Keyed map[string]*sec
	}

	err := gcfg.ReadStringInto(&config{}, "[plain]\ncommon = x\n")
	c.Check(err, check.NotNil)

	cfg := config{}
	err = readWithMapInto(strings.NewReader("[keyed \"k1\"]\n"), map[string]string{
		"PLAIN_COMMON":    "x",
		"KEYED_k1_COMMON": "x",
		"KEYED_k2_COMMON": "x",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{Keyed: map[string]*sec{"k1": {}}})

	cfg = config{}
	err = readWithMapInto(strings.NewReader("[keyed \"k1\"]\n"), map[string]string{
		"PLAIN_FLAG":    "true",
		"KEYED_k1_FLAG": "true",
		"KEYED_k2_FLAG": "true",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{
		Plain: sec{Common: Common{Flag: true}},
		Keyed: map[string]*sec{
			"k1": {Common: Common{Flag: true}},
			"k2": {Common: Common{Flag: true}},
		},
	})
}

func (s *Suite) TestUnparseableEmbeddedTypeNameStillShadows(c *check.C) {
	// An embedded struct that cannot be set by its type name still takes
	// part in name resolution, so a deeper field of the same name stays
	// hidden, as it is in gcfg, which resolves the name to the embedded
	// struct and then fails to parse it.
	type Deep struct {
		Common string
	}
	type Common struct {
		Deep
	}
	type config struct {
		Sec struct {
			Common
		}
	}

	err := gcfg.ReadStringInto(&config{}, "[sec]\ncommon = x\n")
	c.Check(err, check.NotNil)

	cfg := config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{"SEC_COMMON": "x"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Common.Deep.Common, check.Equals, "")
}

// Level is a struct that parses from text, for embedding by type name.
type Level struct {
	Name string
}

func (l *Level) UnmarshalText(text []byte) error {
	l.Name = strings.ToUpper(string(text))
	return nil
}

func (s *Suite) TestEmbeddedParseableStructByTypeName(c *check.C) {
	// Any embedded struct whose type parses from text is set by its type
	// name, not only time.Time, in a plain section, an existing subsection
	// and a new one.
	type sec struct {
		Level
		Other string
	}
	type config struct {
		Plain sec
		Keyed map[string]*sec
	}

	want := config{}
	err := gcfg.ReadStringInto(&want,
		"[plain]\nlevel = high\n[keyed \"k1\"]\nlevel = high\n[keyed \"k2\"]\nlevel = high\n")
	c.Assert(err, check.IsNil)

	cfg := config{}
	err = readWithMapInto(strings.NewReader("[keyed \"k1\"]\n"), map[string]string{
		"PLAIN_LEVEL":    "high",
		"KEYED_k1_LEVEL": "high",
		"KEYED_k2_LEVEL": "high",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, want)
	c.Check(cfg.Plain.Level.Name, check.Equals, "HIGH")
}

func (s *Suite) TestEmbeddedPointerByTypeName(c *check.C) {
	// An embedded pointer to a struct is addressable by its type name on
	// the same terms as an embedded struct: it is set when its type parses
	// from text, and the variable is ignored otherwise. gcfg fails to parse
	// the latter.
	type Common struct {
		Flag bool
	}
	type sec struct {
		*Common
		*Level
	}
	type config struct {
		Sec sec
	}

	err := gcfg.ReadStringInto(&config{}, "[sec]\ncommon = x\n")
	c.Check(err, check.NotNil)

	want := config{}
	err = gcfg.ReadStringInto(&want, "[sec]\nlevel = high\n")
	c.Assert(err, check.IsNil)

	cfg := config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_COMMON": "x",
		"SEC_LEVEL":  "high",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, want)
	c.Check(cfg.Sec.Level, check.DeepEquals, &Level{Name: "HIGH"})
}

func (s *Suite) TestEmbeddedUnexportedTypeNotAddressableByName(c *check.C) {
	// The exception: embedding an unexported type leaves the field itself
	// unsettable, so only its promoted fields can be reached. gcfg agrees,
	// because it requires the field to be settable.
	type sec struct {
		base
		Audience string
	}
	type config struct {
		Sec sec
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), map[string]string{"SEC_BASE": "notset"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{})
}

func (s *Suite) TestUnexportedEmbeddedDoesNotCancelASibling(c *check.C) {
	// An unexported embedded field cannot be set, so it must not occupy the
	// name: a real field that shares it stays reachable. gcfg drops
	// candidates it cannot set before it resolves a name, and so do we.
	type sec struct {
		base
		Base string
	}
	type config struct {
		Sec sec
	}

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, "[sec]\nbase = fromfile\n")
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Base, check.Equals, "fromfile")

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{"SEC_BASE": "set"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Base, check.Equals, "set")
}

func (s *Suite) TestUnexportedEmbeddedDoesNotShadowAPromotedField(c *check.C) {
	type Inner struct {
		Base string
	}
	type sec struct {
		base
		Inner
	}
	type config struct {
		Sec sec
	}

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, "[sec]\nbase = fromfile\n")
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Inner.Base, check.Equals, "fromfile")

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{"SEC_BASE": "set"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Sec.Inner.Base, check.Equals, "set")
}

func (s *Suite) TestUnexportedEmbeddedDoesNotCancelASection(c *check.C) {
	// The same shape one level up, where it would take a whole section with
	// it.
	type commonSec struct {
		Level string
	}
	type common struct {
		Unused string
	}
	type config struct {
		common
		Common commonSec
	}

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, "[common]\nlevel = fromfile\n")
	c.Check(err, check.IsNil)
	c.Check(cfg.Common.Level, check.Equals, "fromfile")

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{"COMMON_LEVEL": "set"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Common.Level, check.Equals, "set")
}

func (s *Suite) TestUnexportedEmbeddedDoesNotCancelASubsectionField(c *check.C) {
	type subsec struct {
		base
		Base string
	}
	type config struct {
		Sec map[string]*subsec
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), map[string]string{"SEC_k_BASE": "set"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Assert(cfg.Sec, check.HasLen, 1)
	c.Check(cfg.Sec["k"].Base, check.Equals, "set")
}

func (s *Suite) TestGcfgTagOptions(c *check.C) {
	// A gcfg tag may carry options after the name, as in
	// `gcfg:"the-name,int=dho"`. Only the part before the comma names the
	// field: including the rest produced a name no environment variable can
	// have, so the field could not be set at all. The option itself has to be
	// honoured too, or the same field would read differently from a file.
	type sec struct {
		Numbered int `gcfg:"the-number,int=dho"`
		Optioned int `gcfg:",int=dho"`
		Plain    int
	}
	type config struct {
		Sec sec
	}

	byFile := config{}
	err := gcfg.ReadStringInto(&byFile, "[sec]\nthe-number = 0777\noptioned = 0777\nplain = 0777\n")
	c.Check(err, check.IsNil)
	c.Check(byFile.Sec, check.DeepEquals, sec{Numbered: 0777, Optioned: 0777, Plain: 777})

	byEnv := config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_THE_NUMBER": "0777",
		"SEC_OPTIONED":   "0777",
		"SEC_PLAIN":      "0777",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Check(byEnv.Sec, check.DeepEquals, byFile.Sec)
}

func (s *Suite) TestGcfgTagOptionsThroughPointersAndSlices(c *check.C) {
	// The field's own int= override has to survive the walk into a pointer's
	// or a slice element's type, where the default for the container type
	// would otherwise take over.
	type sec struct {
		Ptr   *int  `gcfg:"ptr,int=dho"`
		Slice []int `gcfg:"slice,int=dho"`
		Plain []int `gcfg:"plain"`
	}
	type config struct {
		Sec sec
	}

	byFile := config{}
	err := gcfg.ReadStringInto(&byFile, "[sec]\nptr = 0777\nslice = 0777\nplain = 0777\n")
	c.Check(err, check.IsNil)
	c.Assert(byFile.Sec.Ptr, check.NotNil)
	c.Check(*byFile.Sec.Ptr, check.Equals, 0777)
	c.Check(byFile.Sec.Slice, check.DeepEquals, []int{0777})
	c.Check(byFile.Sec.Plain, check.DeepEquals, []int{777})

	byEnv := config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_PTR":   "0777",
		"SEC_SLICE": "0777",
		"SEC_PLAIN": "0777",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Assert(byEnv.Sec.Ptr, check.NotNil)
	c.Check(*byEnv.Sec.Ptr, check.Equals, *byFile.Sec.Ptr)
	c.Check(byEnv.Sec.Slice, check.DeepEquals, byFile.Sec.Slice)
	c.Check(byEnv.Sec.Plain, check.DeepEquals, byFile.Sec.Plain)
}

func (s *Suite) TestSubsectionKeyEndingInAPropertyName(c *check.C) {
	// The key is the variable with the property's name trimmed off the end.
	// Replacing the first match instead would take a bite out of the key.
	type subsec struct {
		URL string
	}
	type config struct {
		Sec map[string]*subsec
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""),
		map[string]string{"SEC_PROD_URL_v2_URL": "https://example.com"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Assert(cfg.Sec, check.HasLen, 1)
	c.Assert(cfg.Sec["PROD_URL_v2"], check.NotNil)
	c.Check(cfg.Sec["PROD_URL_v2"].URL, check.Equals, "https://example.com")
}

func (s *Suite) TestEmbeddedSections(c *check.C) {
	// A section declared on an embedded struct. gcfg resolves a section name
	// by promotion, so it is readable from a file and has to be reachable
	// from the environment as well.
	type logging struct {
		Level string
	}
	type Common struct {
		Logging logging
	}
	type config struct {
		Common
		Server logging
	}

	configString := "[logging]\nlevel = fromfile\n"

	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg.Logging.Level, check.Equals, "fromfile")

	cfg = config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"LOGGING_LEVEL": "set",
		"SERVER_LEVEL":  "set",
	}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg.Logging.Level, check.Equals, "set")
	c.Check(cfg.Server.Level, check.Equals, "set")
}

func (s *Suite) TestDefinedIntUsesTheSameBasesAsGcfg(c *check.C) {
	// gcfg accepts octal for a defined integer type but not for a builtin
	// one, so a leading zero has to mean the same thing in a file and in an
	// environment variable.
	type octInt int
	type sec struct {
		Defined octInt
		Builtin int
	}
	type config struct {
		Sec sec
	}

	byFile := config{}
	err := gcfg.ReadStringInto(&byFile, "[sec]\ndefined = 0755\nbuiltin = 0755\n")
	c.Check(err, check.IsNil)
	c.Check(byFile.Sec.Defined, check.Equals, octInt(0755))
	c.Check(byFile.Sec.Builtin, check.Equals, 755)

	byEnv := config{}
	err = readWithMapInto(strings.NewReader(""), map[string]string{
		"SEC_DEFINED": "0755",
		"SEC_BUILTIN": "0755",
	}, "", &byEnv)
	c.Check(err, check.IsNil)
	c.Check(byEnv.Sec, check.DeepEquals, byFile.Sec)
}

func (s *Suite) TestNameCollisionWithinOneStruct(c *check.C) {
	// Two fields of one struct that map to the same name cancel each other,
	// as they do in gcfg, which reports the name as unstorable rather than
	// picking one.
	type sec struct {
		A string `gcfg:"x"`
		X string
	}
	type config struct {
		Sec sec
	}

	cfg := config{}
	err := readWithMapInto(strings.NewReader(""), map[string]string{"SEC_X": "notset"}, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, config{})
}

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

// parityTagOptionsIndirect keeps the option on a field whose value is reached
// through another type, where the container's own default would otherwise win.
type parityTagOptionsIndirect struct {
	Ptr   *int   `gcfg:"ptr,int=dho"`
	Slice []int  `gcfg:"slice,int=dho"`
	Plain []int  `gcfg:"plain"`
	Text  string `gcfg:"text,int=dho"`
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
	// gcfg tags carrying options, in a section and in a subsection, and on
	// values reached through a pointer or a slice.
	&struct{ Sec parityTagOptions }{},
	&struct {
		Sec map[string]*parityTagOptionsSub
	}{},
	&struct{ Sec parityTagOptionsIndirect }{},
}

// parityValues are tried in order; the first one gcfg accepts is the one
// compared. Covering several kinds keeps a name from being judged unreachable
// merely because the value did not suit its type.
var parityValues = []string{"parityvalue", "7", "true"}

// parityKeys are the subsection names to try for a map-valued section. One of
// them ends in a property name, which is how a key gets mangled if the suffix is
// trimmed carelessly. The empty key is the unnamed subsection, a plain [section]
// heading in a file.
var parityKeys = []string{"k1", "PROD_URL_v2", ""}

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
			if tag := strings.SplitN(sf.Tag.Get("gcfg"), ",", 2)[0]; tag != "" {
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

func Test(t *testing.T) {
	_ = check.Suite(&Suite{})
	check.TestingT(t)
}
