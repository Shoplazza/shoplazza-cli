package ossupload

import (
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

// filePartContentType parses the multipart body produced by buildOSSForm and
// returns the Content-Type declared on the `file` part.
func filePartContentType(t *testing.T, body io.Reader, ct string) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() == "file" {
			return part.Header.Get("Content-Type")
		}
	}
	t.Fatal("no file part in multipart body")
	return ""
}

// TestBuildOSSForm_ContentTypeByExtension verifies a .js artifact is uploaded
// with a JavaScript MIME type while server-consumed artifacts (.wasm, .zip)
// keep the generic binary type.
func TestBuildOSSForm_ContentTypeByExtension(t *testing.T) {
	sr := signResp{Policy: "P", AccessID: "AK", Sign: "SG"}
	cases := []struct {
		fileName string
		want     string
	}{
		{"demo.js", "text/javascript"},
		{"bundle.mjs", "text/javascript"},
		{"func.wasm", "application/octet-stream"},
		{"theme.zip", "application/octet-stream"},
	}
	for _, c := range cases {
		body, ct, err := buildOSSForm(sr, "chick-extension/"+c.fileName, c.fileName, strings.NewReader("x"))
		if err != nil {
			t.Fatalf("%s: %v", c.fileName, err)
		}
		if got := filePartContentType(t, body, ct); got != c.want {
			t.Errorf("%s: file part Content-Type = %q, want %q", c.fileName, got, c.want)
		}
	}
}
