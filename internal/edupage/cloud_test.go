package edupage

import (
	"bytes"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestBuildUploadAttRequest_FieldNaming(t *testing.T) {
	contentType, body, err := buildUploadAttRequest("homework.pdf", strings.NewReader("file contents"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("invalid content type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("mediaType = %q, want multipart/form-data", mediaType)
	}

	mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("reading multipart part: %v", err)
	}
	if part.FormName() != "att" {
		t.Errorf("form field name = %q, want %q", part.FormName(), "att")
	}
	if part.FileName() != "homework.pdf" {
		t.Errorf("filename = %q, want %q", part.FileName(), "homework.pdf")
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(part); err != nil {
		t.Fatalf("reading part contents: %v", err)
	}
	if buf.String() != "file contents" {
		t.Errorf("contents = %q, want %q", buf.String(), "file contents")
	}

	if _, err := mr.NextPart(); err == nil {
		t.Error("expected exactly one multipart part")
	}
}

func TestParseCloudUploadResponse(t *testing.T) {
	t.Run("ok status maps every field", func(t *testing.T) {
		body := []byte(`{"status":"ok","cloudid":"abc123","extension":".pdf","type":"application/pdf","file":"cloud/abc123.pdf","name":"homework.pdf"}`)
		f, err := parseCloudUploadResponse(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.CloudID != "abc123" || f.Extension != ".pdf" || f.FileType != "application/pdf" ||
			f.File != "cloud/abc123.pdf" || f.Name != "homework.pdf" {
			t.Errorf("got %#v", f)
		}
	})

	t.Run("non-ok status is an error", func(t *testing.T) {
		body := []byte(`{"status":"error","error":"file too large"}`)
		_, err := parseCloudUploadResponse(body)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "file too large") {
			t.Errorf("error %q should mention the EduPage error message", err)
		}
	})

	t.Run("missing status is an error", func(t *testing.T) {
		if _, err := parseCloudUploadResponse([]byte(`{}`)); err == nil {
			t.Error("expected an error when status is absent")
		}
	})

	t.Run("invalid JSON is an error", func(t *testing.T) {
		if _, err := parseCloudUploadResponse([]byte("not json")); err == nil {
			t.Error("expected an error for a non-JSON body")
		}
	})
}

func TestCloudFile_URL(t *testing.T) {
	c := &Client{subdomain: "myschool"}

	t.Run("relative path gets base URL and leading slash", func(t *testing.T) {
		f := CloudFile{File: "cloud/abc123.pdf"}
		want := "https://myschool.edupage.org/cloud/abc123.pdf"
		if got := f.URL(c); got != want {
			t.Errorf("URL() = %q, want %q", got, want)
		}
	})

	t.Run("path already has a leading slash", func(t *testing.T) {
		f := CloudFile{File: "/cloud/abc123.pdf"}
		want := "https://myschool.edupage.org/cloud/abc123.pdf"
		if got := f.URL(c); got != want {
			t.Errorf("URL() = %q, want %q", got, want)
		}
	})

	t.Run("already-absolute URL passes through", func(t *testing.T) {
		f := CloudFile{File: "https://elsewhere.example/file.pdf"}
		if got := f.URL(c); got != f.File {
			t.Errorf("URL() = %q, want %q", got, f.File)
		}
	})

	t.Run("empty File yields empty URL", func(t *testing.T) {
		f := CloudFile{}
		if got := f.URL(c); got != "" {
			t.Errorf("URL() = %q, want empty", got)
		}
	})
}

// TestParseCloudUploadResponseNestedData covers the shape EduPage actually
// returns: the file descriptor nested under "data". Reading only the top
// level yields an all-empty CloudFile, which silently attaches a broken
// reference to a message.
func TestParseCloudUploadResponseNestedData(t *testing.T) {
	body := []byte(`{"status":"ok","data":{"cloudid":"289511f51a9ef018765ae5f314d5ddv1","extension":"txt","type":"document","file":"/elearning/ruqjzfpv?z%3Aabc","name":"schalekpage-test.txt"}}`)

	got, err := parseCloudUploadResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CloudID != "289511f51a9ef018765ae5f314d5ddv1" {
		t.Errorf("CloudID = %q", got.CloudID)
	}
	if got.Name != "schalekpage-test.txt" || got.Extension != "txt" || got.FileType != "document" {
		t.Errorf("got %+v", got)
	}
	if got.File == "" {
		t.Error("File must not be empty")
	}
}

// TestParseCloudUploadResponseTopLevel keeps the older flat shape working.
func TestParseCloudUploadResponseTopLevel(t *testing.T) {
	body := []byte(`{"status":"ok","cloudid":"abc123","extension":"pdf","type":"document","file":"/elearning/x","name":"a.pdf"}`)

	got, err := parseCloudUploadResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CloudID != "abc123" || got.Name != "a.pdf" {
		t.Errorf("got %+v", got)
	}
}

// TestParseCloudUploadResponseEmptyDescriptor rejects a success response that
// carries no usable reference, rather than handing back an empty CloudFile.
func TestParseCloudUploadResponseEmptyDescriptor(t *testing.T) {
	if _, err := parseCloudUploadResponse([]byte(`{"status":"ok"}`)); err == nil {
		t.Fatal("expected an error for a success response with no file reference")
	}
}
