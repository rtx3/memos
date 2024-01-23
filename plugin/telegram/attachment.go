package telegram

import (
	"path/filepath"

	"go.uber.org/zap"

	"github.com/usememos/memos/internal/log"
)

type Attachment struct {
	FileName string
	MimeType string
	FileSize int64
	Data     []byte
}

var mimeTypes = map[string]string{
	".jpg": "image/jpeg",
	".png": "image/png",
	".mp4": "video/mp4", // for video note
	".oga": "audio/ogg", // for voice
}

func (b Attachment) GetMimeType() string {
	if b.MimeType != "" {
		return b.MimeType
	}

	mime, ok := mimeTypes[filepath.Ext(b.FileName)]
	if !ok {
		// Handle unknown file extension
		log.Warn("Unknown file type for ", zap.String("filename", b.FileName))

		return "application/octet-stream"
	}

	return mime
}
