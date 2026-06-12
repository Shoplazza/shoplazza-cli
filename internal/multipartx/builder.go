package multipartx

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
)

// Builder constructs multipart/form-data bodies that can be streamed directly
// to client.DoRaw via RawRequest.Headers["Content-Type"] + RawRequest.Data.
type Builder struct {
	buf      *bytes.Buffer
	writer   *multipart.Writer
	finished bool
}

// New returns a fresh Builder.
func New() *Builder {
	buf := bytes.NewBuffer(nil)
	return &Builder{buf: buf, writer: multipart.NewWriter(buf)}
}

// AddField appends a non-file form field.
func (b *Builder) AddField(name, value string) error {
	if b.finished {
		return errors.New("multipartx: Builder already finalized")
	}
	return b.writer.WriteField(name, value)
}

// AddFile appends a file part; the reader is fully drained into the buffer.
// fieldName and filename must NOT contain `"`, `\`, CR, or LF — values are
// inserted directly into the Content-Disposition header.
func (b *Builder) AddFile(fieldName, filename string, reader io.Reader, contentType string) error {
	if b.finished {
		return errors.New("multipartx: Builder already finalized")
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteFormValue(fieldName)+`"; filename="`+quoteFormValue(filename)+`"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	part, err := b.writer.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, reader)
	return err
}

// Build closes the multipart writer and returns the final body reader along
// with the Content-Type (containing the negotiated boundary). The returned
// io.Reader is single-use, as bytes.Buffer.Read advances internal state.
func (b *Builder) Build() (io.Reader, string, error) {
	if b.finished {
		return nil, "", errors.New("multipartx: already built")
	}
	if err := b.writer.Close(); err != nil {
		return nil, "", err
	}
	b.finished = true
	return b.buf, b.writer.FormDataContentType(), nil
}

// quoteFormValue is a placeholder for Content-Disposition value quoting; it
// returns input unchanged. Callers must ensure form-field names and filenames
// do NOT contain `"`, `\`, CR, or LF. To accept arbitrary user-supplied
// filenames, replace this with mime.QEncoding or a quoted-printable encoder.
func quoteFormValue(s string) string {
	return s
}
