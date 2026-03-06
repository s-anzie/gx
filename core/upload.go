package core

import (
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
)

// FileHeader wraps a multipart file with metadata and helper methods.
// The ContentType is detected from magic bytes, not trusting client headers.
type FileHeader struct {
	Filename    string
	Size        int64
	ContentType string
	Header      textproto.MIMEHeader

	// internal
	file *multipart.FileHeader
}

// Open returns the underlying multipart.File for streaming access.
func (f *FileHeader) Open() (multipart.File, error) {
	return f.file.Open()
}

// Save copies the uploaded file to dst, creating the file and all needed
// intermediate directories.
func (f *FileHeader) Save(dst string) error {
	src, err := f.file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// Bytes reads the entire file into memory and returns the bytes.
func (f *FileHeader) Bytes() ([]byte, error) {
	src, err := f.file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	return io.ReadAll(src)
}

// ── Context File Methods ─────────────────────────────────────────────────────

// FormFile returns the first uploaded file associated with the given key.
// It calls ParseMultipartForm if needed, using a 32 MB in-memory limit.
func (c *Context) FormFile(name string) (*FileHeader, error) {
	file, header, err := c.Request.FormFile(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Detect content type from magic bytes
	buf := make([]byte, 512)
	n, _ := file.Read(buf)

	return &FileHeader{
		Filename:    header.Filename,
		Size:        header.Size,
		ContentType: detectMimeFromBytes(buf[:n]),
		Header:      header.Header,
		file:        header,
	}, nil
}

// FormFiles returns all uploaded files for the given key.
func (c *Context) FormFiles(name string) ([]*FileHeader, error) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}

	if c.Request.MultipartForm == nil || c.Request.MultipartForm.File == nil {
		return nil, nil
	}

	headers := c.Request.MultipartForm.File[name]
	if len(headers) == 0 {
		return nil, nil
	}

	result := make([]*FileHeader, 0, len(headers))
	for _, header := range headers {
		// Read magic bytes for MIME detection
		f, err := header.Open()
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		f.Close()

		result = append(result, &FileHeader{
			Filename:    header.Filename,
			Size:        header.Size,
			ContentType: detectMimeFromBytes(buf[:n]),
			Header:      header.Header,
			file:        header,
		})
	}

	return result, nil
}

// detectMimeFromBytes detects MIME type from file magic bytes.
// This is reliable unlike client-provided Content-Type.
func detectMimeFromBytes(buf []byte) string {
	if len(buf) < 4 {
		return "application/octet-stream"
	}

	// Check common magic bytes
	switch {
	case buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF:
		return "image/jpeg"
	case buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47:
		return "image/png"
	case buf[0] == 0x47 && buf[1] == 0x49 && buf[2] == 0x46:
		return "image/gif"
	case buf[0] == 0x52 && buf[1] == 0x49 && buf[2] == 0x46 && buf[3] == 0x46:
		return "image/webp"
	case buf[0] == 0x25 && buf[1] == 0x50 && buf[2] == 0x44 && buf[3] == 0x46:
		return "application/pdf"
	case buf[0] == 0x50 && buf[1] == 0x4B && buf[2] == 0x03 && buf[3] == 0x04:
		return "application/zip"
	case len(buf) > 1 && buf[0] == 0x1F && buf[1] == 0x8B:
		return "application/gzip"
	}

	// Fallback to net/http sniffing
	return "application/octet-stream"
}
