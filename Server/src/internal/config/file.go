package config

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/weirdstreamer/server/internal/fileio"
)

// The config is a YAML file (config.yml) that people edit by hand, with comments, and that the
// server also saves (Settings, the Jobs page, the CLI). A save changes only the values that
// changed: comments, the order of settings, quoting and settings the server doesn't know are kept
// as they were written.

// Load reads the config from path and makes it the current one.
func Load(path string) error {
	data, err := fileio.ReadFile(path)
	if err != nil {
		return err
	}
	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// Ensure each configured media path has a stable ID (for clients)
	for i := range config.Media.Paths {
		if config.Media.Paths[i].ID == "" {
			if abs, err := filepath.Abs(config.Media.Paths[i].Path); err == nil {
				sum := sha1.Sum([]byte(abs))
				config.Media.Paths[i].ID = hex.EncodeToString(sum[:])[:8]
			}
		}
	}
	if err := config.Validate(); err != nil {
		return err
	}

	GlobalConfig = config
	loadedPath = path
	SetTrickplaySettings(config.Jobs.TrickplayRunners, config.Jobs.TrickplayFramesPerRunner)
	SetTrickplayCPUOnly(config.Jobs.TrickplayCPUOnly)
	return nil
}

// Save writes config to path, over what the file holds now (see above). A new file is written
// plain.
func Save(path string, config *Config) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real // a symlinked config stays a symlink
	}
	var doc yaml.Node
	if data, err := fileio.ReadFile(path); err == nil {
		if err := parseKeepingLayout(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return writeOver(path, &doc, config, false)
}

// SaveFrom writes config to path over the file template (config.example.yml): a new config
// with the example's comments. The example's settings that config leaves out stay in it as
// commented-out lines, so their explanations are still there.
func SaveFrom(template, path string, config *Config) error {
	data, err := fileio.ReadFile(template)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := parseKeepingLayout(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", template, err)
	}
	return writeOver(path, &doc, config, true)
}

func writeOver(path string, doc *yaml.Node, config *Config, commentOut bool) error {
	var fresh yaml.Node
	if err := fresh.Encode(config); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&fresh}}
	} else {
		mergeNode(doc.Content[0], &fresh, reflect.TypeOf(*config), commentOut)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	// The blank lines put back are indented like the setting after them.
	out := blankIndent.ReplaceAll(buf.Bytes(), []byte("\n"))
	return writeFileKeepingMode(path, out)
}

var blankIndent = regexp.MustCompile(`(?m)^[ \t]+$`)

// parseKeepingLayout parses data into doc, marking the blank lines between settings to keep:
// yaml.v3 drops them, but writes a head comment's leading newline as one.
func parseKeepingLayout(data []byte, doc *yaml.Node) error {
	if err := yaml.Unmarshal(data, doc); err != nil {
		return err
	}
	markBlankLines(doc, strings.Split(string(data), "\n"))
	return nil
}

func markBlankLines(n *yaml.Node, lines []string) {
	blankBefore := func(holder *yaml.Node, line int) {
		above := line - 1 - commentLines(holder.HeadComment) // the line above it and its comment
		if above >= 1 && above <= len(lines) && strings.TrimSpace(lines[above-1]) == "" {
			holder.HeadComment = "\n" + holder.HeadComment
		}
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !(i == 0 && n.Line == 0) { // see the DocumentNode case
				blankBefore(n.Content[i], n.Content[i].Line)
			}
			markBlankLines(n.Content[i+1], lines)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			// An item's comment may sit on it or on its first key.
			holder := item
			if item.HeadComment == "" && item.Kind == yaml.MappingNode && len(item.Content) > 0 && item.Content[0].HeadComment != "" {
				holder = item.Content[0]
			}
			blankBefore(holder, item.Line)
			markBlankLines(item, lines)
		}
	case yaml.DocumentNode:
		// yaml.v3 keeps the blank line after the file's opening comment itself: the first
		// setting isn't marked (its mapping's line is set to 0 for that).
		for _, c := range n.Content {
			if n.HeadComment != "" && c.Kind == yaml.MappingNode {
				line := c.Line
				c.Line = 0
				markBlankLines(c, lines)
				c.Line = line
				continue
			}
			markBlankLines(c, lines)
		}
	}
}

func commentLines(comment string) int {
	if comment == "" {
		return 0
	}
	return strings.Count(comment, "\n") + 1
}

// writeFileKeepingMode replaces path through a file beside it, so a failed write leaves the old
// config whole, with the old file's permissions (owner-only for a new one: it holds secrets).
func writeFileKeepingMode(path string, data []byte) error {
	mode := os.FileMode(0600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if mode&0o077 == 0 {
		if err := fileio.OwnerOnly(tmp.Name()); err != nil { // Windows: the mode doesn't keep others out
			return err
		}
	}
	return fileio.Rename(tmp.Name(), path)
}

// mergeNode puts fresh's values (the config being saved, for Go type t) into old (the file),
// keeping old's comments and layout. A setting fresh leaves out is removed, or with commentOut
// turned into a comment.
func mergeNode(old, fresh *yaml.Node, t reflect.Type, commentOut bool) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if old.Kind != fresh.Kind {
		replaceNode(old, fresh)
		return
	}
	switch old.Kind {
	case yaml.MappingNode:
		fields := yamlFields(t)
		at := map[string]int{}
		for i := 0; i+1 < len(fresh.Content); i += 2 {
			at[fresh.Content[i].Value] = i
		}
		var content []*yaml.Node
		kept := map[string]bool{}
		pending := "" // settings commented out, for the head comment of the next one kept
		keep := func(key, value *yaml.Node) {
			if pending != "" {
				key.HeadComment = joinComments(pending, key.HeadComment)
				pending = ""
			}
			content = append(content, key, value)
		}
		for i := 0; i+1 < len(old.Content); i += 2 {
			key, value := old.Content[i], old.Content[i+1]
			if j, ok := at[key.Value]; ok {
				mergeNode(value, fresh.Content[j+1], fields[key.Value], commentOut)
				keep(key, value)
				kept[key.Value] = true
				continue
			}
			if _, known := fields[key.Value]; known {
				// A setting that's now empty (left out, as omitempty does)
				if commentOut {
					pending = joinComments(pending, commentedOut(key, value))
				}
				continue
			}
			keep(key, value) // not a setting the server knows: left alone
		}
		for i := 0; i+1 < len(fresh.Content); i += 2 {
			if !kept[fresh.Content[i].Value] {
				keep(fresh.Content[i], fresh.Content[i+1])
			}
		}
		if pending != "" && len(content) > 0 {
			last := content[len(content)-2]
			last.FootComment = joinComments(last.FootComment, pending)
		}
		old.Content = content
	case yaml.SequenceNode:
		var elem reflect.Type
		if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
			elem = t.Elem()
		}
		// Libraries are matched by their id, so removing one doesn't move the others' comments;
		// anything else by position.
		byID := map[string]*yaml.Node{}
		for _, n := range old.Content {
			if id := mappingValue(n, "id"); id != "" {
				byID[id] = n
			}
		}
		used := map[*yaml.Node]bool{}
		content := make([]*yaml.Node, 0, len(fresh.Content))
		for i, n := range fresh.Content {
			match := byID[mappingValue(n, "id")]
			// By position when either has no id (a library not saved yet, or a template's).
			if match == nil && i < len(old.Content) && (mappingValue(old.Content[i], "id") == "" || mappingValue(n, "id") == "") {
				match = old.Content[i]
			}
			if match == nil || used[match] {
				// A new item is quoted like the one before it ("a", "b" stays alike).
				if k := len(content); k > 0 && n.Kind == yaml.ScalarNode && content[k-1].Kind == yaml.ScalarNode && n.Tag == content[k-1].Tag {
					n.Style = content[k-1].Style
				}
				content = append(content, n)
				continue
			}
			used[match] = true
			// A list item's settings that are left out just go: the example's other libraries
			// aren't documentation for this one.
			mergeNode(match, n, elem, false)
			content = append(content, match)
		}
		old.Content = content
	case yaml.ScalarNode:
		if old.Value != fresh.Value || old.ShortTag() != fresh.ShortTag() {
			// The value changes; its comments stay. A string's quoting stays unless it would now
			// read as something else (the encoder quotes it then).
			old.Value, old.Tag = fresh.Value, fresh.Tag
			if old.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 && !strings.Contains(fresh.Value, "\n") {
				old.Style = fresh.Style
			}
		}
	default:
		replaceNode(old, fresh)
	}
}

// commentedOut is a setting as comment lines: its own comment, then "# key: value".
func commentedOut(key, value *yaml.Node) string {
	pair := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key.Value}, value}}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(pair) != nil || enc.Close() != nil {
		return key.HeadComment
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, l := range lines {
		lines[i] = "# " + l
	}
	return joinComments(key.HeadComment, strings.Join(lines, "\n"))
}

// joinComments puts comment b after a. A leading newline (a blank line before, see
// markBlankLines) stays in front.
func joinComments(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "\n" + b
}

// replaceNode gives old fresh's value, keeping old's comments.
func replaceNode(old, fresh *yaml.Node) {
	head, line, foot := old.HeadComment, old.LineComment, old.FootComment
	*old = *fresh
	old.HeadComment, old.LineComment, old.FootComment = head, line, foot
}

// mappingValue is the scalar value of key in a mapping node ("" when it isn't one or has none).
func mappingValue(n *yaml.Node, key string) string {
	if n.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key && n.Content[i+1].Kind == yaml.ScalarNode {
			return n.Content[i+1].Value
		}
	}
	return ""
}

// yamlFields maps a struct's YAML keys to their types.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	if t == nil || t.Kind() != reflect.Struct {
		return fields
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}
	return fields
}
