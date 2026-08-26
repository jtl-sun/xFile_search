package main

import (
	"path/filepath"
	"strings"
)

type Query struct {
	Terms        []string
	NameTerms    []string
	PathTerms    []string
	PathPrefixes []string
	NameGlobs    []string
	Extensions   map[string]struct{}
	ForceFile    bool
	ForceDir     bool
}

func ParseQuery(raw string) Query {
	q := Query{Extensions: make(map[string]struct{})}
	for _, field := range splitQueryFields(strings.TrimSpace(raw)) {
		f := strings.ToLower(strings.TrimSpace(strings.Trim(field, `"`)))
		if f == "" {
			continue
		}
		switch {
		case f == "file:" || f == "file":
			q.ForceFile = true
		case f == "folder:" || f == "folder" || f == "dir:" || f == "dir":
			q.ForceDir = true
		case strings.HasPrefix(f, "ext:"):
			addExtensions(q.Extensions, strings.TrimPrefix(f, "ext:"))
		case strings.HasPrefix(f, "name:"):
			if v := strings.TrimPrefix(f, "name:"); v != "" {
				q.NameTerms = append(q.NameTerms, v)
			}
		case strings.HasPrefix(f, "path:"):
			if v := strings.TrimPrefix(f, "path:"); v != "" {
				q.PathTerms = append(q.PathTerms, normalizePathToken(v))
			}
		case isWindowsPathToken(f):
			parsePathScopedToken(&q, f)
		case isSimpleExtensionGlob(f):
			addExtensions(q.Extensions, strings.TrimPrefix(f, "*."))
		case strings.ContainsAny(f, "*?"):
			q.NameGlobs = append(q.NameGlobs, f)
		default:
			q.Terms = append(q.Terms, f)
		}
	}
	return q
}

// splitQueryFields is strings.Fields with basic quote support so paths such as
// "C:\\Old Designs\\*.jpg" remain one search token.
func splitQueryFields(raw string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range raw {
		switch r {
		case '"':
			quoted = !quoted
		case ' ', '\t', '\r', '\n':
			if quoted {
				b.WriteRune(r)
			} else if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func normalizePathToken(s string) string {
	return strings.ReplaceAll(s, "/", `\`)
}

func isWindowsPathToken(s string) bool {
	s = normalizePathToken(s)
	if len(s) >= 3 && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) && s[1] == ':' && s[2] == '\\' {
		return true
	}
	return strings.HasPrefix(s, `\\`)
}

func isSimpleExtensionGlob(s string) bool {
	return strings.HasPrefix(s, "*.") && len(s) > 2 && !strings.ContainsAny(s[2:], `*?\/`)
}

// parsePathScopedToken makes the common Everything-style form intuitive:
//
//	C:\\*.jpg            -> scope C:\\ + extension jpg
//	S:\\Design\\*.png    -> scope S:\\Design\\ + extension png
//	D:\\ring*.jpg        -> scope D:\\ + filename glob ring*.jpg
//	S:\\jpg              -> scope S:\\ + contains "jpg"
//	S:\\                 -> scope S:\\ only
//
// The path scope is recursive: C:\\Design\\*.jpg includes subfolders.
func parsePathScopedToken(q *Query, token string) {
	token = normalizePathToken(token)
	if strings.HasSuffix(token, `\`) {
		q.PathPrefixes = append(q.PathPrefixes, token)
		return
	}

	lastSlash := strings.LastIndex(token, `\`)
	if lastSlash < 0 {
		q.Terms = append(q.Terms, token)
		return
	}
	prefix := token[:lastSlash+1]
	tail := token[lastSlash+1:]
	if prefix != "" {
		q.PathPrefixes = append(q.PathPrefixes, prefix)
	}
	if tail == "" {
		return
	}
	if isSimpleExtensionGlob(tail) {
		addExtensions(q.Extensions, strings.TrimPrefix(tail, "*."))
		return
	}
	if strings.ContainsAny(tail, "*?") {
		q.NameGlobs = append(q.NameGlobs, tail)
		return
	}
	// A plain tail after a path is treated as a narrowing term. This makes
	// S:\\jpg useful without requiring path:S:\\ + jpg syntax.
	q.Terms = append(q.Terms, tail)
}

func addExtensions(dst map[string]struct{}, raw string) {
	raw = strings.ReplaceAll(raw, ";", ",")
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.TrimPrefix(p, "."))
		if p != "" {
			dst[p] = struct{}{}
		}
	}
}

func (q Query) Empty() bool {
	return len(q.Terms) == 0 && len(q.NameTerms) == 0 && len(q.PathTerms) == 0 && len(q.PathPrefixes) == 0 && len(q.NameGlobs) == 0 && len(q.Extensions) == 0 && !q.ForceFile && !q.ForceDir
}

func (q Query) Match(e Entry, mode FilterMode) bool {
	if mode == FilterFiles && e.IsDir {
		return false
	}
	if mode == FilterFolders && !e.IsDir {
		return false
	}
	if q.ForceFile && e.IsDir {
		return false
	}
	if q.ForceDir && !e.IsDir {
		return false
	}

	for _, prefix := range q.PathPrefixes {
		if !hasPrefixPathFold(e.Path, prefix) {
			return false
		}
	}
	for _, term := range q.NameTerms {
		if !strings.Contains(e.FoldName, term) {
			return false
		}
	}
	for _, glob := range q.NameGlobs {
		if !wildcardMatchFold([]byte(e.Name()), glob) {
			return false
		}
	}
	for _, term := range q.Terms {
		if !containsPathFold(e.Path, term) {
			return false
		}
	}
	for _, term := range q.PathTerms {
		if !containsPathFold(e.Path, term) {
			return false
		}
	}

	if len(q.Extensions) > 0 {
		if e.IsDir {
			return false
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(e.FoldName)), ".")
		if _, ok := q.Extensions[ext]; !ok {
			return false
		}
	}
	return true
}

func (s *IndexSnapshot) MatchAt(id uint32, q Query, mode FilterMode) bool {
	if s == nil {
		return false
	}
	if s.Mapped == nil {
		if int(id) >= len(s.Entries) {
			return false
		}
		return q.Match(s.Entries[id], mode)
	}
	path, nameStart, isDir, ok := s.Mapped.record(id)
	if !ok {
		return false
	}
	if mode == FilterFiles && isDir {
		return false
	}
	if mode == FilterFolders && !isDir {
		return false
	}
	if q.ForceFile && isDir {
		return false
	}
	if q.ForceDir && !isDir {
		return false
	}
	if nameStart < 0 || nameStart > len(path) {
		nameStart = 0
	}
	name := path[nameStart:]
	for _, prefix := range q.PathPrefixes {
		if !hasPrefixBytesFold(path, prefix) {
			return false
		}
	}
	for _, term := range q.NameTerms {
		if !containsBytesFold(name, term) {
			return false
		}
	}
	for _, glob := range q.NameGlobs {
		if !wildcardMatchFold(name, glob) {
			return false
		}
	}
	for _, term := range q.Terms {
		if !containsBytesFold(path, term) {
			return false
		}
	}
	for _, term := range q.PathTerms {
		if !containsBytesFold(path, term) {
			return false
		}
	}
	if len(q.Extensions) > 0 {
		if isDir || !mappedExtensionMatches(name, q.Extensions) {
			return false
		}
	}
	return true
}

func mappedExtensionMatches(name []byte, exts map[string]struct{}) bool {
	dot := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			dot = i
			break
		}
		if name[i] == '\\' || name[i] == '/' {
			break
		}
	}
	if dot < 0 || dot+1 >= len(name) {
		return false
	}
	ext := name[dot+1:]
	for wanted := range exts {
		if equalBytesFoldASCII(ext, wanted) {
			return true
		}
	}
	return false
}

func equalBytesFoldASCII(b []byte, lower string) bool {
	if len(b) != len(lower) {
		return false
	}
	for i := range b {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

func hasPrefixBytesFold(hay []byte, lowerPrefix string) bool {
	if len(lowerPrefix) > len(hay) {
		return false
	}
	for i := 0; i < len(lowerPrefix); i++ {
		a := hay[i]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		b := lowerPrefix[i]
		if b == '/' {
			b = '\\'
		}
		if a == '/' {
			a = '\\'
		}
		if a != b {
			return false
		}
	}
	return true
}

func hasPrefixPathFold(path, lowerPrefix string) bool {
	return hasPrefixBytesFold([]byte(path), lowerPrefix)
}

// wildcardMatchFold supports '*' and '?' on a filename without allocating.
// ASCII matching is case-insensitive; non-ASCII UTF-8 bytes are exact.
func wildcardMatchFold(name []byte, pattern string) bool {
	p := []byte(pattern)
	i, j := 0, 0
	star := -1
	match := 0
	for i < len(name) {
		if j < len(p) && (p[j] == '?' || equalFoldByte(name[i], p[j])) {
			i++
			j++
			continue
		}
		if j < len(p) && p[j] == '*' {
			star = j
			match = i
			j++
			continue
		}
		if star >= 0 {
			j = star + 1
			match++
			i = match
			continue
		}
		return false
	}
	for j < len(p) && p[j] == '*' {
		j++
	}
	return j == len(p)
}

func equalFoldByte(a, b byte) bool {
	if a >= 'A' && a <= 'Z' {
		a += 'a' - 'A'
	}
	if b >= 'A' && b <= 'Z' {
		b += 'a' - 'A'
	}
	return a == b
}

// containsBytesFold is allocation-free and operates directly on the memory-
// mapped path bytes. ASCII is case-insensitive; non-ASCII UTF-8 bytes are
// matched exactly, which is suitable for Korean/Japanese filenames.
func containsBytesFold(hay []byte, lowerNeedle string) bool {
	if lowerNeedle == "" {
		return true
	}
	if len(lowerNeedle) > len(hay) {
		return false
	}
	for i := 0; i+len(lowerNeedle) <= len(hay); i++ {
		ok := true
		for j := 0; j < len(lowerNeedle); j++ {
			a := hay[i+j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			b := lowerNeedle[j]
			if a == '/' {
				a = '\\'
			}
			if b == '/' {
				b = '\\'
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// containsPathFold is the string equivalent used by small in-memory tests.
func containsPathFold(s, lowerNeedle string) bool {
	return containsBytesFold([]byte(s), lowerNeedle)
}
