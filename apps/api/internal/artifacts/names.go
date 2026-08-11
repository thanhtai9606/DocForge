package artifacts

import (
	"fmt"
	"path"
	"strings"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

// DownloadName builds a stable, user-facing filename for an artifact download.
func DownloadName(doc *domain.Document, art *domain.Artifact) string {
	base := "document"
	if doc != nil && doc.Filename != "" {
		base = strings.TrimSuffix(path.Base(doc.Filename), path.Ext(doc.Filename))
		base = sanitize(base)
	}
	if base == "" {
		base = "document"
	}
	kind := "export"
	if art != nil && art.Kind != "" {
		kind = sanitize(art.Kind)
	}
	format := "bin"
	if art != nil && art.Format != "" {
		format = strings.ToLower(art.Format)
	}
	ext := ExtensionForFormat(format)
	if art != nil && art.Kind == "cdom" {
		return fmt.Sprintf("%s.cdom%s", base, ext)
	}
	if kind == "export" {
		return fmt.Sprintf("%s%s", base, ext)
	}
	return fmt.Sprintf("%s.%s%s", base, kind, ext)
}

// ExtensionForFormat maps logical formats to file extensions.
func ExtensionForFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "markdown", "md":
		return ".md"
	case "docx":
		return ".docx"
	case "json":
		return ".json"
	case "pdf":
		return ".pdf"
	default:
		if format == "" {
			return ".bin"
		}
		return "." + sanitize(format)
	}
}

// ContentTypeForFormat returns a download Content-Type.
func ContentTypeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "markdown", "md":
		return "text/markdown; charset=utf-8"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "json":
		return "application/json"
	case "pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func sanitize(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == ' ':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "file"
	}
	return out
}
