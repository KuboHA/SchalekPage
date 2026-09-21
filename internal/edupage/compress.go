package edupage

import (
	"bytes"
	"compress/flate"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

// EncodeRequestBody builds the eqap/eqacs/eqaz form body EduPage's RPC expects.
//
// EduPage's edubarUtils.js compresses the form-encoded payload with raw
// DEFLATE (zlib window bits -15, i.e. no zlib/gzip header) and then
// base64-encodes the compressed bytes using the browser's btoa(), which
// operates on a JS string where each character's code point is one byte
// (0-255) of the compressed data. Go's encoding/base64 (encoding/base64.StdEncoding)
// operates directly on the raw byte slice and produces byte-identical output
// to that latin1-string-based btoa() for any byte sequence — the Python
// reference implementation replicates btoa()/atob() character-by-character
// only because Python str is not byte-oriented; a Go []byte already *is* the
// same "sequence of bytes" the browser's algorithm is defined over, so there
// is no need to replicate the character shuffling here.
func EncodeRequestBody(data map[string]string) string {
	formEncoded := encodeFormData(data)

	compressed, err := deflateRaw([]byte(formEncoded))
	if err != nil {
		// compress/flate cannot fail for a plain byte slice with a valid
		// compression level; keep this defensive rather than panicking.
		compressed = nil
	}

	encoded := base64.StdEncoding.EncodeToString(compressed)
	eqap := "dz:" + encoded

	sum := sha1.Sum([]byte(eqap))
	eqacs := hex.EncodeToString(sum[:])

	return encodeFormData(map[string]string{
		"eqap":  eqap,
		"eqacs": eqacs,
		"eqaz":  "1",
	})
}

// DecodeResponse unwraps an "eqz:"/"eqwd:" prefixed response body.
//
// Notably this only base64-decodes the payload; it does NOT re-inflate it,
// matching the Python reference implementation's (surprising, but verified)
// behaviour: EduPage's RPC responses are sent base64-encoded but not
// compressed, unlike requests.
func DecodeResponse(text string) (string, error) {
	switch {
	case strings.HasPrefix(text, "eqwd:"):
		decoded, err := decodeBase64(text[len("eqwd:"):])
		if err != nil {
			return "", fmt.Errorf("edupage: decode eqwd response: %w", err)
		}
		return string(decoded), nil
	case strings.HasPrefix(text, "eqz:"):
		decoded, err := decodeBase64(text[len("eqz:"):])
		if err != nil {
			return "", fmt.Errorf("edupage: decode eqz response: %w", err)
		}
		return string(decoded), nil
	default:
		return text, nil
	}
}

// deflateRaw compresses data with raw DEFLATE (no zlib/gzip header),
// equivalent to Python's zlib.compressobj(-1, zlib.DEFLATED, -15, ...).
func deflateRaw(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// inflateRaw decompresses raw-DEFLATE data. It is used only by tests to
// verify deflateRaw's output is a valid inverse, mirroring what a real
// EduPage-compatible server would do with a request body (responses are
// never re-inflated by this client; see DecodeResponse).
func inflateRaw(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

// decodeBase64 decodes standard base64, tolerating missing padding the way
// the browser's atob()/our reference implementation's hand-rolled decoder
// does.
func decodeBase64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\f', '\r':
			return -1
		default:
			return r
		}
	}, s)

	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}

// encodeFormData url-encodes data the way EduPage's own JS (and the Python
// reference client) does: percent-encoding matching Python's
// urllib.parse.quote(value, safe="/") rather than Go's url.QueryEscape
// (which escapes '/' and turns spaces into '+'). Keys are sorted for
// deterministic output; EduPage's PHP backend parses form bodies as an
// unordered map, so key order carries no semantic meaning here.
func encodeFormData(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i != 0 {
			b.WriteByte('&')
		}
		b.WriteString(pyQuote(k))
		b.WriteByte('=')
		b.WriteString(pyQuote(data[k]))
	}
	return b.String()
}

// pyQuote percent-encodes s the way Python's urllib.parse.quote(s, safe="/")
// does: unreserved characters (letters, digits, "_.-~") and "/" pass through
// unescaped; everything else is percent-encoded from its UTF-8 bytes.
func pyQuote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isPyUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isPyUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '.' || c == '-' || c == '~' || c == '/':
		return true
	default:
		return false
	}
}
