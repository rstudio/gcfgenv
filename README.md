# gcfgenv

[![Go Reference](https://pkg.go.dev/badge/github.com/rstudio/gcfgenv.svg)](https://pkg.go.dev/github.com/rstudio/gcfgenv)

**This repository is a proof-of-concept meant to serve internal needs only.**

`gcfgenv` is a Go package for configuring applications with environment
variables in addition to their existing configuration files.

The precise goal is to provide environment variable overrides for [`gcfg`
configurations](https://gopkg.in/gcfg.v1) without the need to specify anything
manually whatsoever. There is a strong "convention over configuration" ethos.

`gcfgenv` is **not** a general-purpose way to read environment variables into a
struct -- the package only supports to structures and conventions permitted by
`gcfg`, which are comparatively limited.

## Usage

There are only two exported functions:

* `ReadWithEnvInto()`, which wraps `gcfg.ReadInto()`; and
* `ReadFileWithEnvInto()`, which wraps `gcfg.ReadFileInto()`

Configuration fields are converted to environment variables using the follow
rules:

* Section and field names (including those using [the `gcfg` struct
  tag](https://pkg.go.dev/gopkg.in/gcfg.v1#hdr-Data_structure)) are converted to
  uppercase.
* Slice fields use `,` as a separator.
* Dashes are converted to underscores.
* Subsection names are left as-is.

For example, the following environment variables (and global prefix `APPNAME_`):

``` shell
APPNAME_SEC_FIELD=geese
APPNAME_SEC_k1_FIELD=cats
APPNAME_SEC_k1_OTHER_FIELD=zebras,elephants
```

are equivalent to the following configuration file:

``` ini
[sec]
field = geese

[sec "k1"]
field = cats
other-field = zebras
other-field = elephants
```

## Limitations

* Slice fields that may legitimately contain `,` in their entries cannot be
  parsed correctly.

* No support for setting `gcfg`'s "default values" subsection. It is not
  possible to determine after the initial configuration file pass whether a
  value was defaulted, so resetting a field's default via environment variable
  could lead to surprising results.

* Subsections with whitespace in the subsection heading (i.e. `[Section "Sub Section"]`) require
  environment variable keys that have whitespace. This does not work in most shells or environments.
  We recommend using `snake_case` or `kebab-case` in lieu of whitespace delimited subsection headings.
  A whitespace substitute (i.e. `_` or `-`) is not implemented in environment variable parsing because it leads to
  non-deterministic behavior or unwieldy complexity. Choose your subsection headers wisely.

## Versioning

`gcfgenv` follows semantic versioning.

## License

Licensed under the Apache License, Version 2.0. See `LICENSE` for details.
