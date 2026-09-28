package gonfig

import (
	"bytes"
	"encoding"
	"fmt"
	"io"
	"maps"
	"net"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Format is an output format of Write.
type Format string

const (
	// FormatYAML writes YAML with a comment above every value.
	FormatYAML Format = "yaml"
	// FormatJSON writes indented JSON; JSON has no comments.
	FormatJSON Format = "json"
	// FormatTOML writes TOML with a comment above every value.
	FormatTOML Format = "toml"
	// FormatEnv writes NAME=value lines for environment variables, e.g. for .env
	// files, `docker run --env-file` or `kubectl create configmap --from-env-file`.
	FormatEnv Format = "env"

	// ErrUnknownFormat is returned by Write for a format it does not support.
	ErrUnknownFormat Error = "unknown format"
)

// WriteOption configures Write.
type WriteOption func(*writeOptions)

type writeOptions struct {
	envPrefix string
}

// WithEnvPrefix sets the prefix of environment variable names, the same one as
// Config.EnvPrefix of the loader (without the trailing underscore).
func WithEnvPrefix(prefix string) WriteOption {
	return func(o *writeOptions) { o.envPrefix = prefix }
}

// writer knows the struct tag of a format and how to print its tree.
type writer struct {
	tag    string // struct tag with the key names
	inline string // tag option that inlines a named struct field
	print  func(buf *bytes.Buffer, nodes []*node, prefix string) error
}

var writers = map[Format]writer{
	FormatYAML: {tag: "yaml", inline: fileInlineOption, print: printYAML},
	FormatJSON: {tag: "json", inline: fileInlineOption, print: printJSON},
	FormatTOML: {tag: "toml", inline: fileInlineOption, print: printTOML},
	FormatEnv:  {tag: envTag, inline: envInlineOption, print: printEnv},
}

// Write writes v, a pointer to a struct, in the given format, so that loading the
// output gives the same values back. It follows the rules of the loader: keys
// come from the format's struct tag (the field name without it), embedded
// structs and fields tagged `,inline` (`,squash` for env) are inlined, nil
// pointers are left out, durations, IP networks and encoding.TextMarshaler
// values are written as text. Fields are written in the order they are declared.
//
// A value the format cannot hold is an error: in FormatEnv a line break in a value
// (ErrEnvValue). A value whose MarshalText fails is left out.
//
// Values of `secret:"true"` fields are left empty. YAML, TOML and env output
// have a comment above every value with its `usage`, environment variable and
// `default` tag, so the output of a struct with default values is a documented
// template of the whole configuration.
func Write(w io.Writer, v any, format Format, options ...WriteOption) error {
	out, ok := writers[format]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownFormat, format)
	}

	var opts writeOptions
	for _, option := range options {
		option(&opts)
	}

	root := reflect.ValueOf(v)
	if root.Kind() != reflect.Pointer {
		return fmt.Errorf("%w, got %q", ErrExpectPointer, root.Kind())
	} else if root.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%w, got %q", ErrExpectStruct, root.Elem().Kind())
	}

	walk := walker{tag: out.tag, inline: out.inline, way: []visit{visitOf(root)}}

	var buf bytes.Buffer
	if err := out.print(&buf, walk.fields(root.Elem(), envName{ok: true}, false), opts.envPrefix); err != nil {
		return err
	}

	_, err := w.Write(buf.Bytes())

	return err
}

// node is a field in the output: a value or a section with nested fields.
type node struct {
	key      string
	env      string // environment variable without the prefix; "" if env cannot set it
	usage    string
	def      string // the `default` tag; empty for secrets
	secret   bool
	value    any     // string, bool, int64, uint64, float64, []any or []*node
	children []*node // fields of a section
	section  bool
	entry    bool // an entry of a map: key is data, not a tag
}

// comment describes a value: `usage (env: NAME, default: x)`.
func (n *node) comment(withEnv bool, prefix string) string {
	var meta []string
	if withEnv && n.env != "" {
		meta = append(meta, "env: "+prefixed(prefix, n.env))
	}

	if n.secret {
		meta = append(meta, "secret")
	} else if n.def != "" {
		meta = append(meta, "default: "+n.def)
	}

	switch {
	case len(meta) == 0:
		return n.usage
	case n.usage == "":
		return strings.Join(meta, ", ")
	default:
		return n.usage + " (" + strings.Join(meta, ", ") + ")"
	}
}

func prefixed(prefix, name string) string {
	return envPrefix(prefix) + name
}

// envName is the environment variable of a field, built like the env loader
// does: segments of `env` tags joined by "_"; embedded and `,squash` fields add
// no segment; a field without an `env` tag or with `env:"-"` cannot be set.
type envName struct {
	name string
	ok   bool
}

func (e envName) field(field reflect.StructField) envName {
	parts := strings.Split(field.Tag.Get(envTag), ",")

	switch {
	case !e.ok || parts[0] == "-":
		return envName{}
	case field.Anonymous && derefType(field.Type).Kind() == reflect.Struct,
		parts[0] == "" && slices.Contains(parts[1:], envInlineOption):
		return e
	case parts[0] == "":
		return envName{}
	}

	return e.key(parts[0])
}

func (e envName) key(key string) envName {
	if !e.ok {
		return e
	}

	if e.name == "" {
		return envName{name: key, ok: true}
	}

	return envName{name: e.name + envDelimiter + key, ok: true}
}

// walker builds the output tree of a struct for one format.
type walker struct {
	tag    string
	inline string
	way    []visit // the structs being written, from the root: a pointer back to one is left out
}

// into returns the walker for the value v points to; false for a pointer back to a
// struct being written, a cycle.
func (w walker) into(v reflect.Value) (walker, bool) {
	if v.Kind() != reflect.Pointer {
		return w, true
	}

	if slices.Contains(w.way, visitOf(v)) {
		return w, false
	}

	w.way = append(slices.Clip(w.way), visitOf(v))

	return w, true
}

func (w walker) fields(v reflect.Value, env envName, secret bool) []*node {
	var out []*node

	own := make(map[*node]bool)   // the nodes of fields of v, not of inlined ones
	keys := make(map[string]bool) // their keys, a left out one's too: it would still read into it

	for field, value := range v.Fields() {
		key, inline, skip := w.key(field)
		if skip {
			continue
		}

		fieldSecret := secret || isTrue(field.Tag, SecretTag)
		if inline {
			out = append(out, w.inlined(value, env.field(field), fieldSecret)...)

			continue
		}

		n := &node{key: key, usage: field.Tag.Get(FlagTagUsage), secret: fieldSecret}
		if !fieldSecret {
			n.def = field.Tag.Get(defaultTagName)
		}

		keys[strings.ToLower(key)] = true

		if w.fill(n, value, env.field(field)) {
			out, own[n] = append(out, n), true
		}
	}

	return unshadowed(out, own, keys)
}

// unshadowed leaves out a node of an inlined field whose key a field of the struct
// itself (own, with its keys, even a nil one's), or an earlier inlined field, already
// has: in Go the field of an embedded struct is shadowed by one of the same name, and
// keys match case-insensitively.
func unshadowed(nodes []*node, own map[*node]bool, keys map[string]bool) []*node {
	keys = maps.Clone(keys)

	return slices.DeleteFunc(nodes, func(n *node) bool {
		key := strings.ToLower(n.key)
		if own[n] {
			return false
		}

		shadowed := keys[key]
		keys[key] = true

		return shadowed
	})
}

// inlined returns the nodes of an inlined field: the fields of a struct or of the
// struct a pointer points to, the entries of a map (see inlineHook).
func (w walker) inlined(v reflect.Value, env envName, secret bool) []*node {
	switch {
	case v.Kind() == reflect.Map && !secret: // even the keys of a secret map may tell too much
		return w.entries(v, env, false)
	case v.Kind() == reflect.Struct:
		return w.fields(v, env, secret)
	case v.Kind() == reflect.Pointer && !v.IsNil():
		if inner, ok := w.into(v); ok {
			return inner.fields(v.Elem(), env, secret)
		}
	}

	return nil
}

// key returns the key of a field, whether the field is inlined into its parent,
// or whether it is left out, by the same rules the loader decodes with: a map is
// inlined, a pointer to a map is not (see inlineKeys).
func (w walker) key(field reflect.StructField) (key string, inline, skip bool) {
	parts := strings.Split(field.Tag.Get(w.tag), ",")
	elem := derefType(field.Type)
	isStruct := elem.Kind() == reflect.Struct && !isTextValue(elem)

	switch {
	case parts[0] == "-", ParseTagOptions(field.Tag).FlagArgs: // positional arguments are not configuration
		return "", false, true
	case field.Anonymous && isStruct:
		return "", true, false
	case !field.IsExported():
		return "", false, true
	case slices.Contains(parts[1:], w.inline) &&
		(isStruct || w.inline == fileInlineOption && field.Type.Kind() == reflect.Map):
		return "", true, false
	case parts[0] == "":
		return field.Name, false, false
	}

	return parts[0], false, false
}

// fill sets the value of n and reports whether n has anything to write.
func (w walker) fill(n *node, v reflect.Value, env envName) bool {
	switch {
	case isTextValue(v.Type()):
	case v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface:
		if v.IsNil() {
			return false
		}

		inner, ok := w.into(v)

		return ok && inner.fill(n, v.Elem(), env)
	case v.Kind() == reflect.Struct:
		n.section, n.children = true, w.fields(v, env, n.secret)

		return len(n.children) > 0
	case v.Kind() == reflect.Map:
		if n.secret { // even the keys of a secret map may tell too much
			return false
		}

		n.section, n.children = true, w.entries(v, env, false)

		return len(n.children) > 0 || !v.IsNil() // an empty map is written, as {}: it reads back empty, not nil
	}

	if n.secret {
		v = reflect.Zero(v.Type())
	}

	value, ok := w.value(v)
	n.value, n.env = value, env.name

	if !env.ok {
		n.env = ""
	}

	return ok
}

// entries are the items of a map, in the order of their keys.
func (w walker) entries(v reflect.Value, env envName, secret bool) []*node {
	out := make([]*node, 0, v.Len())

	for key, value := range v.Seq2() {
		name := fmt.Sprint(key.Interface())
		n := &node{key: name, secret: secret, entry: true}

		if w.fill(n, value, env.key(name)) {
			out = append(out, n)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })

	return out
}

// value converts a field value to a plain value the encoders write as is.
func (w walker) value(v reflect.Value) (any, bool) {
	if isTextValue(v.Type()) {
		return textOf(v)
	}

	switch v.Kind() {
	case reflect.String:
		return v.String(), true
	case reflect.Bool:
		return v.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint(), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	case reflect.Slice, reflect.Array:
		return w.list(v), true
	case reflect.Pointer, reflect.Interface:
		inner, ok := w.into(v)
		if v.IsNil() || !ok {
			return nil, false
		}

		return inner.value(v.Elem())
	case reflect.Complex64, reflect.Complex128:
		return strconv.FormatComplex(v.Complex(), 'g', -1, v.Type().Bits()), true
	case reflect.Struct:
		return w.fields(v, envName{}, false), true
	case reflect.Map: // inside a list
		return w.entries(v, envName{}, false), true
	default: // funcs, channels: not a config value
		return nil, false
	}
}

func (w walker) list(v reflect.Value) []any {
	out := make([]any, 0, v.Len())

	for i := range v.Len() {
		if value, ok := w.value(v.Index(i)); ok {
			out = append(out, value)
		}
	}

	return out
}

var (
	durationType = reflect.TypeFor[time.Duration]()
	ipNetType    = reflect.TypeFor[net.IPNet]()
	marshalType  = reflect.TypeFor[encoding.TextMarshaler]()
)

// isTextValue reports whether values of t are written as text: the loader reads
// them from a string with the shared hooks. A pointer or an interface is not: the
// value it holds is written.
func isTextValue(t reflect.Type) bool {
	return t.Kind() != reflect.Pointer && t.Kind() != reflect.Interface &&
		(t == durationType || t == ipNetType || t.Implements(marshalType) || reflect.PointerTo(t).Implements(marshalType))
}

// textOf returns the text of a value; an empty IP network is left out, as the
// loader cannot read it back from an empty string.
func textOf(v reflect.Value) (string, bool) {
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)

	switch value := ptr.Interface().(type) {
	case *time.Duration:
		return value.String(), true
	case *net.IPNet:
		return value.String(), value.IP != nil
	default:
		text, err := value.(encoding.TextMarshaler).MarshalText()

		return string(text), err == nil
	}
}
