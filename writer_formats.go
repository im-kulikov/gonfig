package gonfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The printers get a tree of plain values (see node.value) and write into a
// bytes.Buffer, so they cannot fail.

func printYAML(buf *bytes.Buffer, nodes []*node, prefix string) {
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)

	_ = enc.Encode(yamlMapping(nodes, prefix)) // a tree of plain values always encodes
	_ = enc.Close()
}

func yamlMapping(nodes []*node, prefix string) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	for _, n := range nodes {
		value := n.value
		if n.section {
			value = n.children
		}

		key := &yaml.Node{Kind: yaml.ScalarNode, Value: n.key, HeadComment: n.comment(true, prefix)}
		mapping.Content = append(mapping.Content, key, yamlValue(value, prefix))
	}

	return mapping
}

func yamlValue(value any, prefix string) *yaml.Node {
	switch v := value.(type) {
	case []*node:
		return yamlMapping(v, prefix)
	case []any:
		list := &yaml.Node{Kind: yaml.SequenceNode}
		if len(v) == 0 {
			list.Style = yaml.FlowStyle // []
		}

		for _, item := range v {
			list.Content = append(list.Content, yamlValue(item, prefix))
		}

		return list
	default:
		node := new(yaml.Node)
		_ = node.Encode(v) // string, bool or a number

		return node
	}
}

func printJSON(buf *bytes.Buffer, nodes []*node, _ string) {
	var compact bytes.Buffer
	jsonValue(&compact, nodes)

	_ = json.Indent(buf, compact.Bytes(), "", "  ") // jsonValue writes valid JSON
	buf.WriteByte('\n')
}

// jsonValue writes objects field by field, in their order: a map would sort them.
func jsonValue(buf *bytes.Buffer, value any) {
	switch v := value.(type) {
	case []*node:
		buf.WriteByte('{')

		for i, n := range v {
			if i > 0 {
				buf.WriteByte(',')
			}

			jsonValue(buf, n.key)
			buf.WriteByte(':')

			if n.section {
				jsonValue(buf, n.children)
			} else {
				jsonValue(buf, n.value)
			}
		}

		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')

		for i, item := range v {
			if i > 0 {
				buf.WriteByte(',')
			}

			jsonValue(buf, item)
		}

		buf.WriteByte(']')
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			jsonValue(buf, strconv.FormatFloat(v, 'g', -1, 64)) // JSON has no NaN; the loader parses "NaN"
		} else {
			buf.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		}
	default: // string, bool, int64, uint64
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false) // keep URLs readable: no & for &

		_ = enc.Encode(v)
		buf.Truncate(buf.Len() - 1) // Encode ends with a newline
	}
}

// printTOML writes the values of a table before its sub-tables, as TOML requires.
func printTOML(buf *bytes.Buffer, nodes []*node, prefix string) {
	tomlTable(buf, nodes, nil, prefix)
}

func tomlTable(buf *bytes.Buffer, nodes []*node, path []string, prefix string) {
	for _, n := range nodes {
		if !n.section {
			writeComment(buf, n.comment(true, prefix))
			fmt.Fprintf(buf, "%s = %s\n", tomlKey(n.key), tomlValue(n.value))
		}
	}

	for _, n := range nodes {
		if n.section {
			table := append(slices.Clone(path), tomlKey(n.key))

			buf.WriteByte('\n')
			writeComment(buf, n.comment(true, prefix))
			fmt.Fprintf(buf, "[%s]\n", strings.Join(table, "."))
			tomlTable(buf, n.children, table, prefix)
		}
	}
}

func tomlValue(value any) string {
	switch v := value.(type) {
	case []*node: // a struct inside a list: an inline table
		items := make([]string, 0, len(v))

		for _, n := range v {
			item := n.value
			if n.section {
				item = n.children
			}

			items = append(items, tomlKey(n.key)+" = "+tomlValue(item))
		}

		return "{ " + strings.Join(items, ", ") + " }"
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, tomlValue(item))
		}

		return "[" + strings.Join(items, ", ") + "]"
	case float64:
		switch {
		case math.IsNaN(v):
			return "nan"
		case math.IsInf(v, 1):
			return "inf"
		case math.IsInf(v, -1):
			return "-inf"
		}

		return strconv.FormatFloat(v, 'g', -1, 64)
	default: // strings, bools and integers are written in TOML as in JSON
		var buf bytes.Buffer
		jsonValue(&buf, v)

		return buf.String()
	}
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func tomlKey(key string) string {
	if bareKey.MatchString(key) {
		return key
	}

	return tomlValue(key)
}

// printEnv writes NAME=value for every value env can set, sections flattened.
func printEnv(buf *bytes.Buffer, nodes []*node, prefix string) {
	for _, n := range nodes {
		if n.section {
			printEnv(buf, n.children, prefix)

			continue
		}

		if value, ok := envValue(n.value); ok && n.env != "" {
			writeComment(buf, n.comment(false, prefix))
			fmt.Fprintf(buf, "%s=%s\n", prefixed(prefix, n.env), value)
		}
	}
}

// envValue formats a value as the env loader reads it: lists are comma-separated,
// structs inside lists cannot be set from env.
func envValue(value any) (string, bool) {
	switch v := value.(type) {
	case []*node:
		return "", false
	case []any:
		items := make([]string, 0, len(v))

		for _, item := range v {
			text, ok := envValue(item)
			if !ok {
				return "", false
			}

			items = append(items, text)
		}

		return strings.Join(items, ","), true
	default:
		return fmt.Sprint(v), true
	}
}

func writeComment(buf *bytes.Buffer, comment string) {
	if comment != "" {
		fmt.Fprintf(buf, "# %s\n", comment)
	}
}
