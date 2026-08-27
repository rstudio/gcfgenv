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
* Anonymous embedded structs are flattened, matching how `gcfg` reads them from
  a file. Both a section and a field can be declared on an embedded struct and
  named as though it had been declared on the outer one. An exported embedded
  struct is *also* addressable by its own type name, which is what makes an
  embedded `time.Time` usable; embedding an unexported type is the exception,
  since the field itself is not settable, so only its promoted fields can be
  reached.
* Where two fields compete for one name, the name resolves as it does in Go: a
  field at a shallower depth wins, and otherwise neither is reachable. This
  applies to a `gcfg` tag that collides with a sibling field's name as well as
  to embedding.

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

The following are known places where an environment variable and a file entry
still disagree. None of them has a caller today, and each needs a piece of
`gcfg`'s setter machinery that this package does not reproduce:

* A `big.Int` field is parsed by `math/big`, which infers the base from the
  literal, rather than by `gcfg`'s decimal-and-hexadecimal rule. `0777` is 511
  from an environment variable and 777 from a file.

* A `uintptr` field cannot be set at all, and the attempt fails the whole read
  as an unsupported type, though `gcfg` reads one from a file.

* A property or section promoted through an *embedded pointer* to a struct is
  not reachable, because traversing one would panic when the pointer is nil.

* When two fields compete for a name, this package resolves the competition on
  the environment variable name, whereas `gcfg` resolves it on the Go field
  name. Two fields whose Go names collide but whose `gcfg` tags do not are
  refused by `gcfg` and settable here.

* The `Default_<Section>` struct is matched by Go field name, where `gcfg` folds
  `default-<section name>`. A section renamed by a `gcfg` tag therefore takes
  its defaults from one and not the other.

## Versioning

`gcfgenv` follows semantic versioning.

## License

Licensed under the Apache License, Version 2.0. See `LICENSE` for details.
