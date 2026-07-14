// Dockerfile detector: FROM lines with ARG substitution, multi-stage
// awareness, line continuations, and per-tag base-OS decomposition.
package detect

import (
	"strings"
)

// dockerfile extracts declarations from every FROM instruction.
//
// It resolves the pieces that matter for an offline audit and no more:
// ARG defaults declared before the first FROM substitute into image
// references (${VER}, ${VER:-3.9}, $VER); references to earlier build
// stages and FROM scratch are skipped; --platform flags are ignored.
func (e *Engine) dockerfile(content []byte) []Decl {
	var decls []Decl
	args := map[string]string{}
	stages := map[string]bool{}
	for _, ins := range dockerfileInstructions(content) {
		fields := strings.Fields(ins.text)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "ARG":
			// "ARG NAME=default"; a bare "ARG NAME" has no build-time
			// value we can know offline, so it stays unresolved.
			for _, f := range fields[1:] {
				if name, val, ok := strings.Cut(f, "="); ok {
					args[name] = strings.Trim(val, `"'`)
				}
			}
		case "FROM":
			rest := fields[1:]
			for len(rest) > 0 && strings.HasPrefix(rest[0], "--") {
				rest = rest[1:] // --platform and future flags
			}
			if len(rest) == 0 {
				continue
			}
			raw := rest[0]
			if len(rest) >= 3 && strings.EqualFold(rest[1], "AS") {
				stages[strings.ToLower(rest[2])] = true
			}
			image := expandArgs(raw, args)
			if strings.EqualFold(image, "scratch") || stages[strings.ToLower(image)] {
				continue // empty base or a reference to an earlier stage
			}
			decls = append(decls, e.resolveImage(ParseImageRef(image), raw, ins.line, "dockerfile")...)
			// The stage name of the *current* FROM only becomes skippable
			// after this instruction, which the map ordering already gives us.
		}
	}
	return decls
}

// instruction is one logical Dockerfile instruction with the line number
// of its first physical line.
type instruction struct {
	text string
	line int
}

// dockerfileInstructions joins backslash continuations into logical
// instructions and drops comment lines, preserving the starting line
// number of each instruction for accurate findings.
func dockerfileInstructions(content []byte) []instruction {
	var out []instruction
	var buf strings.Builder
	start := 0
	flush := func() {
		text := strings.TrimSpace(buf.String())
		if text != "" {
			out = append(out, instruction{text: text, line: start})
		}
		buf.Reset()
		start = 0
	}
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(trimmed, "#") {
			continue // comments may appear inside a continuation block
		}
		if buf.Len() == 0 {
			if trimmed == "" {
				continue
			}
			start = i + 1
		}
		if strings.HasSuffix(trimmed, "\\") {
			buf.WriteString(strings.TrimSuffix(trimmed, "\\"))
			buf.WriteString(" ")
			continue
		}
		buf.WriteString(trimmed)
		flush()
	}
	flush()
	return out
}

// expandArgs substitutes ${NAME}, ${NAME:-default}, and $NAME using ARG
// defaults collected so far. Unknown variables are left in place so the
// caller can flag the reference as unresolved rather than guessing.
func expandArgs(s string, args map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			out.WriteByte(s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				out.WriteString(s[i:])
				break
			}
			expr := s[i+2 : i+2+end]
			name, def, hasDef := strings.Cut(expr, ":-")
			if val, ok := args[name]; ok && val != "" {
				out.WriteString(val)
			} else if hasDef {
				out.WriteString(def)
			} else {
				out.WriteString(s[i : i+3+end]) // keep ${NAME} verbatim
			}
			i += 3 + end
			continue
		}
		// $NAME — the name runs while characters are [A-Za-z0-9_].
		j := i + 1
		for j < len(s) && isVarChar(s[j]) {
			j++
		}
		if j == i+1 {
			out.WriteByte('$')
			i++
			continue
		}
		if val, ok := args[s[i+1:j]]; ok && val != "" {
			out.WriteString(val)
		} else {
			out.WriteString(s[i:j])
		}
		i = j
	}
	return out.String()
}

func isVarChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
