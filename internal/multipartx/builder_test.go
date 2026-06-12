package multipartx

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseMultipart(t *testing.T, body io.Reader, contentType string) (map[string]string, map[string][]byte) {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	mr := multipart.NewReader(body, params["boundary"])
	fields := map[string]string{}
	files := map[string][]byte{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		buf := bytes.NewBuffer(nil)
		_, _ = io.Copy(buf, part)
		filename := part.FileName()
		if filename == "" {
			fields[part.FormName()] = buf.String()
		} else {
			files[part.FormName()] = buf.Bytes()
		}
		_ = textproto.MIMEHeader(part.Header)
	}
	return fields, files
}

func TestBuilder_SingleFileMultipleFields(t *testing.T) {
	b := New()
	if err := b.AddField("name", "NoirChic"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddField("version", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddFile("file", "theme.zip", strings.NewReader("ZIP-CONTENT"), "application/zip"); err != nil {
		t.Fatal(err)
	}
	body, ct, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Errorf("Content-Type: %q", ct)
	}
	fields, files := parseMultipart(t, body, ct)
	if fields["name"] != "NoirChic" {
		t.Errorf("field name = %q", fields["name"])
	}
	if fields["version"] != "1.0.0" {
		t.Errorf("field version = %q", fields["version"])
	}
	if string(files["file"]) != "ZIP-CONTENT" {
		t.Errorf("file content = %q", files["file"])
	}
}

func TestBuilder_BuildAfterBuildFails(t *testing.T) {
	b := New()
	_, _, _ = b.Build()
	if err := b.AddField("k", "v"); err == nil {
		t.Fatal("AddField after Build should fail")
	}
}

func TestBuilder_EmptyFieldsAndEmptyFile(t *testing.T) {
	b := New()
	if err := b.AddField("emptyField", ""); err != nil {
		t.Fatal(err)
	}
	if err := b.AddFile("empty", "empty.txt", strings.NewReader(""), "text/plain"); err != nil {
		t.Fatal(err)
	}
	body, ct, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	fields, files := parseMultipart(t, body, ct)
	if v, ok := fields["emptyField"]; !ok || v != "" {
		t.Errorf("empty field missing or wrong value: present=%v value=%q", ok, v)
	}
	if data, ok := files["empty"]; !ok || len(data) != 0 {
		t.Errorf("empty file part missing or non-empty: present=%v len=%d", ok, len(data))
	}
}

func TestFileFormBody_SingleFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.zip")
	_ = os.WriteFile(tmp, []byte("ZIP"), 0o644)
	body, ct, err := FileFormBody("file", tmp, "application/zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, files := parseMultipart(t, body, ct)
	if string(files["file"]) != "ZIP" {
		t.Errorf("file content: %q", files["file"])
	}
}

func TestFileFormBody_WithExtraFields(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "x.bin")
	_ = os.WriteFile(tmp, []byte("data"), 0o644)
	body, ct, _ := FileFormBody("file", tmp, "application/octet-stream", map[string]string{
		"name":    "x",
		"version": "1",
	})
	fields, files := parseMultipart(t, body, ct)
	if fields["name"] != "x" || fields["version"] != "1" {
		t.Errorf("extra fields: %v", fields)
	}
	if string(files["file"]) != "data" {
		t.Errorf("file content: %q", files["file"])
	}
}

func TestBuilder_BoundaryStable(t *testing.T) {
	// Verify boundary chosen at New() time stays stable through field additions and Build.
	b := New()
	beforeBoundary := b.writer.Boundary()
	if beforeBoundary == "" {
		t.Fatal("multipart.Writer.Boundary() returned empty before any operations")
	}
	if err := b.AddField("k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddField("k2", "v2"); err != nil {
		t.Fatal(err)
	}
	_, ct, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	// Content-Type contains boundary as "boundary=<val>"
	want := "boundary=" + beforeBoundary
	if !strings.Contains(ct, want) {
		t.Fatalf("boundary changed: Content-Type=%q does not contain %q", ct, want)
	}
}

func TestBuilder_PartsArriveInInsertionOrder(t *testing.T) {
	// Insertion order: name → version → file. Wire order must match.
	b := New()
	_ = b.AddField("name", "NoirChic")
	_ = b.AddField("version", "1.0.0")
	_ = b.AddFile("file", "theme.zip", strings.NewReader("ZIP"), "application/zip")
	body, ct, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	_, params, _ := mime.ParseMediaType(ct)
	mr := multipart.NewReader(body, params["boundary"])
	var got []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, part.FormName())
	}
	want := []string{"name", "version", "file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("part order: got %v, want %v", got, want)
	}
}
