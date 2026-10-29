package filestore

import (
	"bytes"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// renderYAML renders doc for a file whose current bytes are original. Where
// the file's shape allows it, only the records and fields that changed are
// rewritten and every other byte of original is kept: quoting, comments, key
// order and indentation of hand-maintained files survive a command write
// (GAP-022). The result must decode to exactly what a whole-file render
// would; when splicing is not possible or that check fails, the whole file is
// rendered as before.
func renderYAML(original []byte, doc any) ([]byte, error) {
	whole, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if len(original) == 0 {
		return whole, nil
	}
	// First keep keys the domain types do not model, so a write does not
	// drop them; if the result does not check, a key was cleared on purpose
	// and is removed.
	for _, keepUnknown := range []bool{true, false} {
		spliced, ok := spliceYAML(original, doc, keepUnknown)
		if ok && sameDocument(spliced, whole, doc) {
			return spliced, nil
		}
	}
	return whole, nil
}

// sameDocument reports whether spliced decodes, as doc's type, to the same
// value that whole renders.
func sameDocument(spliced, whole []byte, doc any) bool {
	target := reflect.New(reflect.TypeOf(doc))
	if err := yaml.Unmarshal(spliced, target.Interface()); err != nil {
		return false
	}
	again, err := yaml.Marshal(target.Elem().Interface())
	return err == nil && bytes.Equal(again, whole)
}

// lineEdit replaces lines [start, end) of the original with text.
type lineEdit struct {
	start, end int
	text       []string
}

type splicer struct {
	lines       []string
	edits       []lineEdit
	keepUnknown bool
}

// insert adds text before line at; insertions at the same line keep their
// order.
func (sp *splicer) insert(at int, text []string) {
	if n := len(sp.edits); n > 0 && sp.edits[n-1].start == at && sp.edits[n-1].end == at {
		sp.edits[n-1].text = append(sp.edits[n-1].text, text...)
		return
	}
	sp.edits = append(sp.edits, lineEdit{start: at, end: at, text: text})
}

func spliceYAML(original []byte, doc any, keepUnknown bool) ([]byte, bool) {
	var root yaml.Node
	if err := yaml.Unmarshal(original, &root); err != nil {
		return nil, false
	}
	var rendered yaml.Node
	if err := rendered.Encode(doc); err != nil {
		return nil, false
	}
	top, next := documentMapping(&root), documentMapping(&rendered)
	if top == nil || next == nil {
		return nil, false
	}
	sp := &splicer{lines: splitLines(original), keepUnknown: keepUnknown}
	for i := 0; i+1 < len(next.Content); i += 2 {
		key, value := next.Content[i].Value, next.Content[i+1]
		at := mappingIndex(top, key)
		if at < 0 {
			return nil, false
		}
		old := top.Content[at+1]
		if equalNodes(old, value) {
			continue
		}
		limit := sp.siblingLimit(top, at)
		var ok bool
		switch {
		case old.Kind == yaml.SequenceNode && value.Kind == yaml.SequenceNode:
			ok = sp.sequence(key, old, value, limit)
		case old.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode:
			ok = sp.mapping(key, old, value, limit, false)
		}
		if !ok {
			return nil, false
		}
	}
	return sp.apply(), true
}

// sequence splices the records of a block sequence, matched by id.
func (sp *splicer) sequence(key string, old, next *yaml.Node, limit int) bool {
	if old.Style&yaml.FlowStyle != 0 || len(old.Content) == 0 {
		return false
	}
	oldIDs, ok := recordIDs(old)
	if !ok {
		return false
	}
	newIDs, ok := recordIDs(next)
	if !ok {
		return false
	}
	starts := make([]int, len(old.Content))
	for i, item := range old.Content {
		starts[i] = item.Line - 1
		if starts[i] < 0 || starts[i] >= len(sp.lines) || !strings.HasPrefix(strings.TrimLeft(sp.lines[starts[i]], " "), "- ") {
			return false
		}
	}
	dash := len(sp.lines[starts[0]]) - len(strings.TrimLeft(sp.lines[starts[0]], " "))
	segment := func(i int) (int, int) {
		end := limit
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		return starts[i], sp.contentEnd(starts[i], end, dash)
	}
	byID := map[string]int{}
	for i, id := range oldIDs {
		byID[id] = i
	}
	kept := map[int]bool{}
	last, insertAt := -1, starts[0]
	for j, id := range newIDs {
		i, found := byID[id]
		if !found {
			text, ok := renderSequenceItem(key, nil, next.Content[j], dash)
			if !ok {
				return false
			}
			sp.insert(insertAt, text)
			continue
		}
		if i < last {
			return false
		}
		last = i
		kept[i] = true
		start, end := segment(i)
		insertAt = end
		if equalNodes(old.Content[i], next.Content[j]) {
			continue
		}
		// Change only the fields that differ; when the record does not allow
		// it, render the whole record.
		mark := len(sp.edits)
		if sp.mapping(key, old.Content[i], next.Content[j], end, true) {
			continue
		}
		sp.edits = sp.edits[:mark]
		text, ok := renderSequenceItem(key, old.Content[i], next.Content[j], dash)
		if !ok {
			return false
		}
		sp.edits = append(sp.edits, lineEdit{start: start, end: end, text: text})
	}
	for i := range old.Content {
		if !kept[i] {
			start, end := segment(i)
			sp.edits = append(sp.edits, lineEdit{start: start, end: end})
		}
	}
	return true
}

// mapping splices the entries of a block mapping, matched by key, down to
// nested block mappings. onDash marks a sequence item, whose first entry
// shares its line with the dash and is never rewritten here.
func (sp *splicer) mapping(key string, old, next *yaml.Node, limit int, onDash bool) bool {
	if old.Style&yaml.FlowStyle != 0 || len(old.Content) == 0 {
		return false
	}
	column := old.Content[0].Column - 1
	segment := func(at int) (int, int) {
		end := limit
		if at+2 < len(old.Content) {
			end = old.Content[at+2].Line - 1
		}
		start := old.Content[at].Line - 1
		return start, sp.contentEnd(start, end, column)
	}
	for i := 0; i < len(old.Content); i += 2 {
		if old.Content[i].Line-1 < 0 || old.Content[i].Column-1 != column {
			return false
		}
	}
	kept := map[int]bool{}
	last, insertAt := -1, old.Content[0].Line-1
	for j := 0; j+1 < len(next.Content); j += 2 {
		name := next.Content[j].Value
		at := mappingIndex(old, name)
		if at < 0 {
			if onDash && last < 0 {
				return false
			}
			text, ok := renderMappingEntry(key, nil, next.Content[j], next.Content[j+1], column)
			if !ok {
				return false
			}
			sp.insert(insertAt, text)
			continue
		}
		if at < last {
			return false
		}
		last = at
		kept[at] = true
		start, end := segment(at)
		insertAt = end
		if equalNodes(old.Content[at+1], next.Content[j+1]) {
			continue
		}
		if onDash && at == 0 {
			return false
		}
		oldValue, newValue := old.Content[at+1], next.Content[j+1]
		if oldValue.Kind == yaml.MappingNode && newValue.Kind == yaml.MappingNode && oldValue.Line > old.Content[at].Line {
			mark := len(sp.edits)
			if sp.mapping(key, oldValue, newValue, end, false) {
				continue
			}
			sp.edits = sp.edits[:mark]
		}
		text, ok := renderMappingEntry(key, oldValue, next.Content[j], newValue, column)
		if !ok {
			return false
		}
		sp.edits = append(sp.edits, lineEdit{start: start, end: end, text: text})
	}
	for at := 0; at < len(old.Content); at += 2 {
		if !kept[at] && !sp.keepUnknown {
			if onDash && at == 0 {
				return false
			}
			start, end := segment(at)
			sp.edits = append(sp.edits, lineEdit{start: start, end: end})
		}
	}
	return true
}

// siblingLimit is the first line after the value of the entry at index at of
// mapping top: the next entry's key line, or the end of the file.
func (sp *splicer) siblingLimit(top *yaml.Node, at int) int {
	if at+2 < len(top.Content) {
		return top.Content[at+2].Line - 1
	}
	return len(sp.lines)
}

// contentEnd drops trailing blank lines and comment lines that are not
// indented deeper than the record from [start, end): they belong to what
// follows and stay where they are.
func (sp *splicer) contentEnd(start, end, column int) int {
	for end > start+1 {
		line := sp.lines[end-1]
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && indent <= column) {
			end--
			continue
		}
		break
	}
	return end
}

func (sp *splicer) apply() []byte {
	// Edits never overlap; apply them from the end so line numbers hold.
	edits := append([]lineEdit(nil), sp.edits...)
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && (edits[j].start > edits[j-1].start || (edits[j].start == edits[j-1].start && edits[j].end > edits[j-1].end)); j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
	lines := append([]string(nil), sp.lines...)
	for _, edit := range edits {
		tail := append([]string(nil), lines[edit.end:]...)
		lines = append(append(lines[:edit.start], edit.text...), tail...)
	}
	return []byte(strings.Join(lines, ""))
}

// renderSequenceItem renders one record as a block sequence item with its
// dash at column dash, keeping the styles and comments of old where values
// did not change.
func renderSequenceItem(key string, old, next *yaml.Node, dash int) ([]string, bool) {
	item := keepStyles(old, next)
	wrapper := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{item}},
	}}
	return renderBelow(wrapper, dash)
}

// renderMappingEntry renders one key and value of a block mapping with the
// key at column column.
func renderMappingEntry(key string, old, name, next *yaml.Node, column int) ([]string, bool) {
	value := keepStyles(old, next)
	wrapper := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{name, value}},
	}}
	return renderBelow(wrapper, column)
}

// renderBelow renders wrapper, drops its first line (the wrapping key) and
// moves the rest so its first line starts at column.
func renderBelow(wrapper *yaml.Node, column int) ([]string, bool) {
	out, err := yaml.Marshal(wrapper)
	if err != nil {
		return nil, false
	}
	lines := splitLines(out)
	if len(lines) < 2 {
		return nil, false
	}
	lines = lines[1:]
	shift := column - (len(lines[0]) - len(strings.TrimLeft(lines[0], " ")))
	for i, line := range lines {
		switch {
		case strings.TrimSpace(line) == "":
		case shift > 0:
			lines[i] = strings.Repeat(" ", shift) + line
		case shift < 0:
			if !strings.HasPrefix(line, strings.Repeat(" ", -shift)) {
				return nil, false
			}
			lines[i] = line[-shift:]
		}
	}
	return lines, true
}

// keepStyles returns next with the scalar styles, tags and comments of old
// wherever old has the same value at the same place.
func keepStyles(old, next *yaml.Node) *yaml.Node {
	if old == nil || old.Kind != next.Kind {
		return next
	}
	next.HeadComment, next.LineComment, next.FootComment = old.HeadComment, old.LineComment, old.FootComment
	switch next.Kind {
	case yaml.ScalarNode:
		if old.Value == next.Value {
			next.Style, next.Tag = old.Style, old.Tag
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(next.Content); i += 2 {
			if at := mappingIndex(old, next.Content[i].Value); at >= 0 {
				keepStyles(old.Content[at], next.Content[i])
				next.Content[i+1] = keepStyles(old.Content[at+1], next.Content[i+1])
			}
		}
	case yaml.SequenceNode:
		for i := range next.Content {
			if i < len(old.Content) {
				next.Content[i] = keepStyles(old.Content[i], next.Content[i])
			}
		}
	}
	return next
}

// equalNodes compares two nodes by value: scalar text, mapping entries in
// any order and sequence items in order; styles, tags and comments do not
// count.
func equalNodes(a, b *yaml.Node) bool {
	for a.Kind == yaml.AliasNode && a.Alias != nil {
		a = a.Alias
	}
	for b.Kind == yaml.AliasNode && b.Alias != nil {
		b = b.Alias
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.Value == b.Value
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			at := mappingIndex(b, a.Content[i].Value)
			if at < 0 || !equalNodes(a.Content[i+1], b.Content[at+1]) {
				return false
			}
		}
		return true
	case yaml.SequenceNode, yaml.DocumentNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !equalNodes(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// recordIDs returns the id of every item of a sequence of mappings, or false
// when an item is not a mapping, has no id, or an id repeats.
func recordIDs(seq *yaml.Node) ([]string, bool) {
	ids := make([]string, 0, len(seq.Content))
	seen := map[string]bool{}
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			return nil, false
		}
		at := mappingIndex(item, "id")
		if at < 0 {
			return nil, false
		}
		id := item.Content[at+1].Value
		if id == "" || seen[id] {
			return nil, false
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, true
}

func documentMapping(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// mappingIndex returns the index of key's key node in a mapping, or -1.
func mappingIndex(node *yaml.Node, key string) int {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// splitLines splits data into lines that keep their newline; a last line
// without one gets it, so spliced text never runs into the next line.
func splitLines(data []byte) []string {
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
