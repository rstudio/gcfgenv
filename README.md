# gcfgenv

[![Go Reference](https://pkg.go.dev/badge/github.com/rstudio/gcfgenv.svg)](https://pkg.go.dev/github.com/rstudio/gcfgenv)

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
* Slice fields are appended to rather than replaced (as with the original `gcfg`
  package).
* Dashes are converted to underscores.
* Subsection names are left as-is.
* Anonymous embedded structs are flattened, as in `gcfg`. A section or field
  declared on an embedded struct is named as though it were declared on the
  outer struct. An exported embedded struct is also addressable by its type
  name; an unexported one is reachable only through its fields.
* Where two names compete, the shallower wins, and otherwise neither is
  reachable, as in Go.
* Only the part of a `gcfg` tag before the first comma names the field. Options
  after it, such as `int=dho`, are honored.

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

* Modifying subsections with whitespace in the heading (i.e. `[Section "Sub
  Section"]`) requires using environment variables with whitespace, since any
  form of automatic substitution (with e.g. `_` or `-`) would lead to ambiguity.
  However, most shells and other tools do not handle whitespace in environment
  variables well, so we recommend using `snake_case` or `kebab-case` in
  subsection headings instead.

Known cases where an environment variable and a file entry disagree:

* A `big.Int` field infers its base, so `0777` is 511 from an environment
  variable and 777 from a file.

* A `uintptr` field cannot be set, and the attempt fails the read.

* A property or section promoted through an embedded pointer is not reachable.

* Competing names are resolved on the environment variable name rather than the
  Go field name, so two fields whose Go names collide but whose tags do not are
  settable here and refused by `gcfg`.

* `Default_<Section>` is matched by Go field name, where `gcfg` folds
  `default-<section name>`. A section renamed by a tag takes its defaults from
  one and not the other.

## Versioning

`gcfgenv` follows semantic versioning.

## License

Licensed under the Apache License, Version 2.0. See `LICENSE` for details.
