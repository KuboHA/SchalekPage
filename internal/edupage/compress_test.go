package edupage

import (
	"net/url"
	"strings"
	"testing"
)

func TestDeflateRawRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"hello world",
		"eqap=dz%3Asome&eqacs=abc123&eqaz=1",
		strings.Repeat("EduPage compression round trip test. ", 50),
	}

	for _, in := range cases {
		compressed, err := deflateRaw([]byte(in))
		if err != nil {
			t.Fatalf("deflateRaw(%q): %v", in, err)
		}
		out, err := inflateRaw(compressed)
		if err != nil {
			t.Fatalf("inflateRaw of deflateRaw(%q): %v", in, err)
		}
		if string(out) != in {
			t.Fatalf("round trip mismatch: got %q, want %q", out, in)
		}
	}
}

func TestEncodeRequestBodyShape(t *testing.T) {
	body := EncodeRequestBody(map[string]string{"username": "alice", "edupage": ""})

	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("EncodeRequestBody produced unparseable form body: %v", err)
	}

	for _, key := range []string{"eqap", "eqacs", "eqaz"} {
		if _, ok := values[key]; !ok {
			t.Errorf("missing %q in encoded body: %s", key, body)
		}
	}

	if got := values.Get("eqaz"); got != "1" {
		t.Errorf("eqaz = %q, want \"1\"", got)
	}
	if !strings.HasPrefix(values.Get("eqap"), "dz:") {
		t.Errorf("eqap = %q, want dz: prefix", values.Get("eqap"))
	}

	// eqacs must be the sha1 hex digest of eqap (40 hex chars).
	eqacs := values.Get("eqacs")
	if len(eqacs) != 40 {
		t.Errorf("eqacs length = %d, want 40 (sha1 hex)", len(eqacs))
	}
}

func TestEncodeRequestBodyDeterministic(t *testing.T) {
	data := map[string]string{"a": "1", "b": "2", "c": "3"}
	first := EncodeRequestBody(data)
	for i := 0; i < 5; i++ {
		if got := EncodeRequestBody(data); got != first {
			t.Fatalf("EncodeRequestBody is not deterministic: %q != %q", got, first)
		}
	}
}

func TestDecodeResponsePassthrough(t *testing.T) {
	got, err := DecodeResponse("plain response, no envelope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plain response, no envelope" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeResponseEqzAndEqwd(t *testing.T) {
	// "hello" base64-encoded, as EduPage would send it in an eqz:/eqwd:
	// wrapped response. Per the reference implementation, DecodeResponse
	// only base64-decodes; it does not re-inflate.
	const b64 = "aGVsbG8=" // base64("hello")

	got, err := DecodeResponse("eqz:" + b64)
	if err != nil {
		t.Fatalf("eqz: %v", err)
	}
	if got != "hello" {
		t.Fatalf("eqz decoded = %q, want %q", got, "hello")
	}

	got, err = DecodeResponse("eqwd:" + b64)
	if err != nil {
		t.Fatalf("eqwd: %v", err)
	}
	if got != "hello" {
		t.Fatalf("eqwd decoded = %q, want %q", got, "hello")
	}
}

func TestDecodeResponseTolerantOfMissingPadding(t *testing.T) {
	// base64("hello") without its trailing '=' padding.
	got, err := DecodeResponse("eqz:aGVsbG8")
	if err != nil {
		t.Fatalf("unexpected error decoding unpadded base64: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestPyQuoteMatchesPythonSemantics(t *testing.T) {
	cases := map[string]string{
		"hello world":   "hello%20world",
		"a/b":           "a/b",
		"a+b":           "a%2Bb",
		"{\"a\":1}":     "%7B%22a%22%3A1%7D",
		"abcXYZ012_.-~": "abcXYZ012_.-~",
	}
	for in, want := range cases {
		if got := pyQuote(in); got != want {
			t.Errorf("pyQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeFormDataOrderingIsSorted(t *testing.T) {
	got := encodeFormData(map[string]string{"z": "1", "a": "2"})
	want := "a=2&z=1"
	if got != want {
		t.Errorf("encodeFormData = %q, want %q", got, want)
	}
}
