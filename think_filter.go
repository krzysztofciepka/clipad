package main

import "strings"

// thinkFilter removes reasoning-model chain-of-thought spans (delimited by
// <think>...</think>) from a streamed token sequence. It is stateful so that
// the delimiters may be split across arbitrary chunk boundaries: any trailing
// bytes that could be the start of a delimiter are held back until the next
// chunk (or flush) disambiguates them. Leading whitespace before the first
// real output is trimmed so the result starts at the actual answer.
type thinkFilter struct {
	inThink bool
	buf     string
	started bool
}

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// feed consumes a chunk of content and returns the portion safe to emit now.
func (f *thinkFilter) feed(s string) string {
	f.buf += s
	var out strings.Builder
	for {
		if !f.inThink {
			if i := strings.Index(f.buf, thinkOpen); i >= 0 {
				f.write(&out, f.buf[:i])
				f.buf = f.buf[i+len(thinkOpen):]
				f.inThink = true
				continue
			}
			// No full open tag; hold back a trailing partial-tag candidate.
			keep := partialTagSuffix(f.buf, thinkOpen)
			f.write(&out, f.buf[:len(f.buf)-keep])
			f.buf = f.buf[len(f.buf)-keep:]
			return out.String()
		}
		if i := strings.Index(f.buf, thinkClose); i >= 0 {
			f.buf = f.buf[i+len(thinkClose):]
			f.inThink = false
			continue
		}
		// Still inside think; discard all but a trailing close-tag candidate.
		keep := partialTagSuffix(f.buf, thinkClose)
		f.buf = f.buf[len(f.buf)-keep:]
		return out.String()
	}
}

// flush returns any held-back real content at end of stream. Held bytes are
// emitted as literal text (they never completed a tag); an unterminated think
// block is dropped.
func (f *thinkFilter) flush() string {
	defer func() { f.buf = "" }()
	if f.inThink {
		return ""
	}
	var out strings.Builder
	f.write(&out, f.buf)
	return out.String()
}

// write appends s, trimming leading whitespace until the first real output.
func (f *thinkFilter) write(out *strings.Builder, s string) {
	if !f.started {
		s = strings.TrimLeft(s, " \t\r\n")
		if s == "" {
			return
		}
		f.started = true
	}
	out.WriteString(s)
}

// partialTagSuffix returns the length of the longest proper prefix of tag that
// is also a suffix of s (0 if none) — i.e. how many trailing bytes of s might
// be the beginning of tag and must be held back.
func partialTagSuffix(s, tag string) int {
	max := len(tag) - 1
	if max > len(s) {
		max = len(s)
	}
	for k := max; k > 0; k-- {
		if strings.HasSuffix(s, tag[:k]) {
			return k
		}
	}
	return 0
}
