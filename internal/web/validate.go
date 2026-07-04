package web

import (
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

var webLog = slog.With("service", "web")

var allowedMIMETypes = map[string]bool{
	"application/pdf":                                          true,
	"image/jpeg":                                               true,
	"image/png":                                                true,
	"image/gif":                                                true,
	"image/webp":                                               true,
	"image/tiff":                                               true,
	"text/plain":                                               true,
	"text/csv":                                                 true,
	"application/rtf":                                          true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":  true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":        true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/msword":                                       true,
	"application/vnd.ms-excel":                                 true,
	"application/vnd.ms-powerpoint":                            true,
	"application/vnd.oasis.opendocument.text":                  true,
	"application/vnd.oasis.opendocument.spreadsheet":           true,
	"application/vnd.oasis.opendocument.presentation":          true,
	"application/zip":                                          true,
	"application/gzip":                                         true,
	"application/x-7z-compressed":                              true,
	"application/x-tar":                                        true,
}

var dangerousExtensions = map[string]bool{
	".exe": true, ".dll": true, ".bat": true, ".cmd": true,
	".com": true, ".msi": true, ".scr": true, ".pif": true,
	".vbs": true, ".js": true, ".jar": true, ".class": true,
	".sh": true, ".php": true, ".py": true, ".pl": true,
	".rb": true, ".swf": true, ".hta": true, ".cpl": true,
	".reg": true, ".wsf": true, ".wsh": true, ".ps1": true,
	".psm1": true, ".psd1": true, ".iso": true, ".app": true,
	".gadget": true, ".msp": true, ".scf": true, ".lnk": true,
	".inf": true, ".sct": true, ".vb": true, ".jse": true,
	".vbe": true, ".msh": true, ".mshxml": true,
}

type validatedFile struct {
	header      *multipart.FileHeader
	content     io.ReadCloser
	detectedMIME string
}

func validateAttachment(fileHeader *multipart.FileHeader) (*validatedFile, error) {
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if dangerousExtensions[ext] {
		return nil, fmt.Errorf("file type %q is not allowed", ext)
	}

	f, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open attachment: %w", err)
	}

	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]

	detected := http.DetectContentType(buf)

	if !allowedMIMETypes[detected] {
		webLog.Warn("attachment with unallowed MIME type rejected", "filename", fileHeader.Filename, "detected", detected, "provided", fileHeader.Header.Get("Content-Type"))
		f.Close()
		return nil, fmt.Errorf("file type %q is not allowed for attachments", detected)
	}

	vf := &validatedFile{
		header:       fileHeader,
		content:      newMultiReadCloser(f, buf),
		detectedMIME: detected,
	}

	return vf, nil
}

type multiReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func newMultiReadCloser(r io.ReadCloser, prefix []byte) *multiReadCloser {
	return &multiReadCloser{
		reader: io.MultiReader(strings.NewReader(string(prefix)), r),
		closer: r,
	}
}

func (m *multiReadCloser) Read(p []byte) (int, error) {
	return m.reader.Read(p)
}

func (m *multiReadCloser) Close() error {
	return m.closer.Close()
}
