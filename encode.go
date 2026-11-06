package main

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Hand written encoders for the handful of shapes this service emits. They
// append to a caller supplied buffer and do not use reflection, which keeps
// the hot paths free of per-request garbage.

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON string literal, escaping exactly like
// encoding/json does by default (including HTML-sensitive characters, U+2028
// and U+2029, and replacing invalid UTF-8 with U+FFFD).
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '"', '\\':
				b = append(b, '\\', c)
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}

// appendJSONHeaders appends h as a JSON object with sorted keys, matching
// what encoding/json produces for an http.Header.
func appendJSONHeaders(b []byte, keys []string, h map[string][]string) []byte {
	b = append(b, '{')
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendJSONString(b, k)
		b = append(b, ':')
		vs := h[k]
		if vs == nil {
			b = append(b, "null"...)
			continue
		}
		b = append(b, '[')
		for j, v := range vs {
			if j > 0 {
				b = append(b, ',')
			}
			b = appendJSONString(b, v)
		}
		b = append(b, ']')
	}
	return append(b, '}')
}

// appendYAMLString appends s as a YAML scalar. It is written plain when that
// is unambiguous, and double quoted otherwise. The quoting rules follow
// github.com/goccy/go-yaml (which this service used to use) so output is
// unchanged for ordinary values, and additionally quote anything a plain
// scalar cannot represent.
func appendYAMLString(b []byte, s string) []byte {
	if yamlNeedsQuote(s) {
		return strconv.AppendQuote(b, s)
	}
	return append(b, s...)
}

var yamlReserved = map[string]struct{}{
	// null
	"null": {}, "Null": {}, "NULL": {}, "~": {},
	// bool
	"true": {}, "True": {}, "TRUE": {}, "false": {}, "False": {}, "FALSE": {},
	// YAML 1.1 bool
	"y": {}, "Y": {}, "yes": {}, "Yes": {}, "YES": {},
	"n": {}, "N": {}, "no": {}, "No": {}, "NO": {},
	"on": {}, "On": {}, "ON": {}, "off": {}, "Off": {}, "OFF": {},
	// float specials
	".inf": {}, ".Inf": {}, ".INF": {}, "-.inf": {}, "-.Inf": {}, "-.INF": {},
	"+.inf": {}, "+.Inf": {}, "+.INF": {}, ".nan": {}, ".NaN": {}, ".NAN": {},
}

func yamlNeedsQuote(s string) bool {
	if s == "" || s == "-" {
		return true
	}
	switch s[0] {
	case '*', '&', '[', '{', '}', ']', ',', '!', '|', '>', '%', '\'', '"', '@', ' ', '`':
		return true
	case '?':
		if len(s) == 1 || s[1] == ' ' {
			return true
		}
	}
	switch s[len(s)-1] {
	case ':', ' ':
		return true
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '#', '\\':
			return true
		case ':', '-':
			if i+1 < len(s) && s[i+1] == ' ' {
				return true
			}
		default:
			if c < 0x20 || c == 0x7f {
				return true
			}
			if c >= utf8.RuneSelf {
				r, size := utf8.DecodeRuneInString(s[i:])
				if (r == utf8.RuneError && size == 1) || !strconv.IsPrint(r) || r == '\ufeff' {
					return true
				}
				i += size - 1
			}
		}
	}
	if _, ok := yamlReserved[s]; ok {
		return true
	}
	return yamlLooksNumeric(s) || yamlLooksTimestamp(s)
}

// yamlLooksNumeric reports whether a plain scalar s could be read back as a
// number by a YAML 1.1 or 1.2 parser.
func yamlLooksNumeric(s string) bool {
	c := s[0]
	if (c < '0' || c > '9') && c != '+' && c != '-' && c != '.' {
		return false // cheap reject for the common case
	}
	if strings.HasPrefix(s, "_") || strings.Count(s, ".") > 1 {
		// goccy/go-yaml treats these as strings, but versions like
		// "1.2.3" are clearly not numbers anyway.
		return false
	}
	neg := strings.HasPrefix(s, "-")
	n := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(s, "+"), "-"), "_", "")
	base := 10
	switch {
	case strings.HasPrefix(n, "0x"):
		n, base = n[2:], 16
	case strings.HasPrefix(n, "0o"):
		n, base = n[2:], 8
	case strings.HasPrefix(n, "0b"):
		n, base = n[2:], 2
	case strings.Contains(n, ".") || strings.ContainsAny(n, "eE"):
		_, err := strconv.ParseFloat(n, 64)
		return err == nil || isRangeErr(err)
	case len(n) > 1 && n[0] == '0':
		base = 8
	}
	var err error
	if neg {
		_, err = strconv.ParseInt("-"+n, base, 64)
	} else {
		_, err = strconv.ParseUint(n, base, 64)
	}
	return err == nil || isRangeErr(err)
}

func isRangeErr(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

var yamlTimestampFormats = []string{
	time.RFC3339Nano,
	"2006-01-02t15:04:05.999999999Z07:00",
	time.DateTime,
	time.DateOnly,
	"15:4",
}

func yamlLooksTimestamp(s string) bool {
	if c := s[0]; c < '0' || c > '9' {
		return false
	}
	for _, f := range yamlTimestampFormats {
		if _, err := time.Parse(f, s); err == nil {
			return true
		}
	}
	return false
}
