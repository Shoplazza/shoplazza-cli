package multipartx

import (
	"io"
	"os"
	"path/filepath"
)

// FileFormBody opens filePath, adds it as the named file part plus any extra
// string fields, and returns the completed body and Content-Type.
func FileFormBody(fieldName, filePath, contentType string, fields map[string]string) (io.Reader, string, error) {
	b := New()
	for k, v := range fields {
		if err := b.AddField(k, v); err != nil {
			return nil, "", err
		}
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	if err := b.AddFile(fieldName, filepath.Base(filePath), f, contentType); err != nil {
		return nil, "", err
	}
	return b.Build()
}
