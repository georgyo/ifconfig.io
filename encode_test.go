package main

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

var encodeSamples = []string{
	"", "a", "curl/8.0", `"<x>" & ü`, "\x00\x01\x1f\x7f", "\b\f\n\r\t",
	"\xff\xfe", "a b c", "日本語", "\\", "</script>",
}

// checkJSONString compares appendJSONString with encoding/json. Valid UTF-8
// must encode identically. Go versions disagree on whether invalid UTF-8
// becomes a literal U+FFFD or a \ufffd escape, so for those only the decoded
// value has to match.
func checkJSONString(t *testing.T, s string) {
	t.Helper()
	want, _ := json.Marshal(s)
	got := appendJSONString(nil, s)
	if utf8.ValidString(s) {
		if string(got) != string(want) {
			t.Errorf("appendJSONString(%q) = %s, want %s", s, got, want)
		}
		return
	}
	var g, w string
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("appendJSONString(%q) = %s: %v", s, got, err)
	}
	json.Unmarshal(want, &w)
	if g != w {
		t.Errorf("appendJSONString(%q) decodes to %q, want %q", s, g, w)
	}
}

func TestAppendJSONStringMatchesEncodingJSON(t *testing.T) {
	for _, s := range encodeSamples {
		checkJSONString(t, s)
	}
}

func FuzzAppendJSONString(f *testing.F) {
	for _, s := range encodeSamples {
		f.Add(s)
	}
	f.Fuzz(checkJSONString)
}

func TestAppendJSONHeaders(t *testing.T) {
	h := map[string][]string{"B": {"1", "<2>"}, "A": {}, "C": nil}
	want, _ := json.Marshal(h)
	if got := appendJSONHeaders(nil, []string{"A", "B", "C"}, h); string(got) != string(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestAppendYAMLString(t *testing.T) {
	for in, want := range map[string]string{
		// Plain
		"US":                     "US",
		"203.0.113.7":            "203.0.113.7",
		"2001:db8::1":            "2001:db8::1",
		"::1":                    "::1",
		"1.1.1.1, 2.2.2.2":       "1.1.1.1, 2.2.2.2",
		"gzip, deflate, br":      "gzip, deflate, br",
		"en-US,en;q=0.9":         "en-US,en;q=0.9",
		`curl/8.0 "<x>" & ü`:     `curl/8.0 "<x>" & ü`,
		"http://example.com/a?b": "http://example.com/a?b",
		"1.2.3":                  "1.2.3",
		"-foo":                   "-foo",
		"GET":                    "GET",
		// Quoted
		"":           `""`,
		"*/*":        `"*/*"`,
		"NO":         `"NO"`,
		"yes":        `"yes"`,
		"null":       `"null"`,
		"~":          `"~"`,
		"123":        `"123"`,
		"0x1F":       `"0x1F"`,
		"1.5":        `"1.5"`,
		"1e5":        `"1e5"`,
		".nan":       `".nan"`,
		"2001-12-14": `"2001-12-14"`,
		"12:30":      `"12:30"`,
		"a: b":       `"a: b"`,
		"- a":        `"- a"`,
		"a #b":       `"a #b"`,
		"x ":         `"x "`,
		" x":         `" x"`,
		"2001:db8::": `"2001:db8::"`,
		"?":          `"?"`,
		"a\tb":       `"a\tb"`,
		"\x7f":       `"\x7f"`,
		"\xff":       `"\xff"`,
		"a\\b":       `"a\\b"`,
		"@x":         `"@x"`,
	} {
		if got := string(appendYAMLString(nil, in)); got != want {
			t.Errorf("appendYAMLString(%q) = %s, want %s", in, got, want)
		}
	}
}
