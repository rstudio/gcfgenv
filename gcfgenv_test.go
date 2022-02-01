package gcfgenv

import (
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
		c.Check(got.Interface(), check.Equals, tc.want.Interface(),
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

type sec1 struct {
	F1      bool
	F2      int
	F3      float32
	F4      string
	F5      *int
	private string
}

type config struct {
	Sec1    sec1
	Top     int
	private string
}

var (
	configString = `[sec1]
f1 = true
f2 = 25
f3 = 3.12
f4 = value
f5 = 3
`
	configFilled = config{
		Sec1: sec1{
			F1: true,
			F2: 25,
			F3: 3.12,
			F4: "value",
			F5: new(int),
		},
	}

	configEnvVars = map[string]string{
		"SEC1_F2":      "1",
		"SEC1_F5":      "1",
		"SEC1_PRIVATE": "notset",
	}

	configFilledWithEnvVars = config{
		Sec1: sec1{
			F1: true,
			F2: 1,
			F3: 3.12,
			F4: "value",
			F5: new(int),
		},
	}
)

func init() {
	*configFilled.Sec1.F5 = 3
	*configFilledWithEnvVars.Sec1.F5 = 1
}

func (s *Suite) TestUpstream(c *check.C) {
	cfg := config{}
	err := gcfg.ReadStringInto(&cfg, configString)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilled)
}

func (s *Suite) TestReadInto(c *check.C) {
	cfg := config{}
	r := strings.NewReader(configString)
	err := readWithMapInto(r, configEnvVars, "", &cfg)
	c.Check(err, check.IsNil)
	c.Check(cfg, check.DeepEquals, configFilledWithEnvVars)
}

func Test(t *testing.T) {
	_ = check.Suite(&Suite{})
	check.TestingT(t)
}
