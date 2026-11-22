package mailstore

import "mime"

// InlineContentType returns the safe image type to render inline.
func InlineContentType(raw string) (string, bool) {
	switch ct, _, _ := mime.ParseMediaType(raw); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon":
		return ct, true
	}
	return "application/octet-stream", false
}
