package inbound

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"html"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

const maxTextAttachment = 1 << 20
const maxScanBody = 1 << 20
const maxFindings = 64
const maxB64Block = 32 << 10
const maxB64Total = 128 << 10

var severityRank = map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}

type signal struct {
	code, severity string
	pattern        *regexp.Regexp
	markers        []string
}

func rule(code, severity, pattern string, markers ...string) signal {
	return signal{code, severity, regexp.MustCompile(pattern), markers}
}

// RE2 signatures report evidence only: scanning does not invoke any command or interpreter.
var signatures = []signal{
	rule("shell_curl_pipe_sh", "high", `(?im)\bcurl\b[^\r\n|]{0,512}\|\s*(?:sudo\s+)?(?:[\w/.-]+/)?(?:ba)?sh\b`, "curl"),
	rule("shell_wget_pipe_sh", "high", `(?im)\bwget\b[^\r\n|]{0,512}\|\s*(?:sudo\s+)?(?:[\w/.-]+/)?(?:ba)?sh\b`, "wget"),
	rule("shell_remote_exec", "high", `(?i)\b(?:iex|invoke-expression)\s*\([^\r\n]{0,48}\b(?:iwr|irm|invoke-webrequest|invoke-restmethod)\b`, "iex", "invoke-expression"),
	rule("powershell_encoded", "high", `(?i)\b(?:powershell(?:\.exe)?|pwsh)\b[ \t\w.\-]{0,200}\s-(?:enc|encodedcommand)\b`, "powershell", "pwsh"),
	rule("shell_encoded_payload", "high", `(?is)(?:\bbase64\b[^\r\n|]{0,64}\s(?:-d|--decode)\b[^\r\n|]{0,64}\|\s*(?:[\w/.-]+/)?(?:ba)?sh\b|\bfrombase64string\b.{0,384}\b(?:iex|invoke-expression)\b)`, "base64", "frombase64string"),
	rule("js_eval_obfuscated", "high", `(?i)\beval\s*\(\s*(?:atob|unescape|String\.fromCharCode)\s*\(`, "eval"),
	rule("js_eval_obfuscated", "medium", `(?i)\beval\s*\([^\r\n]{0,160}(?:\\x[0-9a-f]{2}){12,}`, "eval"),
	rule("shell_remote_exec", "medium", `(?im)(?:\breg(?:\.exe)?\s+add\b[^\r\n]{0,256}\\(?:run|runonce)\b|\bschtasks(?:\.exe)?\s+/create\b|\|\s*crontab\s+-\s*$|(?:>>?|tee(?:\s+-a)?)\s*['"]?(?:/etc/cron\.|/etc/crontab\b)|\blaunchctl\s+(?:load|bootstrap|enable)\b)`, "reg", "schtasks", "crontab", "/etc/cron.", "launchctl"),
	rule("shell_remote_exec", "high", `(?im)(?:/dev/tcp/|\b(?:nc|ncat|netcat)\b[^\r\n]{0,120}\s-e\s|\bbash\s+-i\s*(?:>&|\|&))`, "/dev/tcp/", "nc", "ncat", "netcat", "bash"),
	rule("shell_destructive_rm", "high", `(?im)\brm\s+-[rfRF]{2,5}\s+(?:--\s+)?/(?:\*|\s|$)`, "rm"),
	rule("shell_remote_exec", "high", `(?im)(?:\bchmod\s+777\b[^\r\n]{0,200}(?:\bcurl\b|\bwget\b|&&\s*\./)|\b(?:curl|wget)\b[^\r\n]{0,200}\bchmod\s+777\b)`, "chmod"),
	rule("shell_remote_exec", "high", `(?im)(?:\bcertutil(?:\.exe)?\s+-urlcache\b|\bmshta(?:\.exe)?\s+(?:https?:|javascript:|vbscript:)|\bregsvr32(?:\.exe)?\b[^\r\n]{0,160}/i:\s*https?:)`, "certutil", "mshta", "regsvr32"),
}
var bodyBlock = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z0-9+/]{16,}={0,2})[ \t\r]*$`)

func clippedBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
func excerpt(text string) string {
	var clean strings.Builder
	n := 0
	for _, c := range text {
		if unicode.IsControl(c) || unicode.In(c, unicode.Cf) {
			continue
		}
		escaped := html.EscapeString(string(c))
		step := utf8.RuneCountInString(escaped)
		if n+step > 200 {
			break
		}
		n += step
		clean.WriteString(escaped)
	}
	return clean.String()
}

type riskScan struct{ a *Analysis }

func (s riskScan) add(code, level string, index, line int, text string) {
	r := s.a.Risk
	if severityRank[level] > severityRank[r.Level] {
		r.Level = level
	}
	f := Finding{Code: code, Severity: level, Location: "body", Line: line, Excerpt: excerpt(text)}
	if index >= 0 {
		f.Location = "attachment"
		f.AttachmentIndex = &index
		if level == "high" && !slices.Contains(s.a.HighRiskParts, index) {
			s.a.HighRiskParts = append(s.a.HighRiskParts, index)
		}
	}
	for _, old := range r.Findings {
		if old.Code == code && old.Severity == level && old.Location == f.Location && old.Line == line && old.Excerpt == f.Excerpt && ((index < 0 && old.AttachmentIndex == nil) || (index >= 0 && old.AttachmentIndex != nil && *old.AttachmentIndex == index)) {
			return
		}
	}
	if len(r.Findings) < maxFindings {
		r.Findings = append(r.Findings, f)
		return
	}
	// Keep higher-severity evidence even if low-level diagnostics filled the cap.
	for i := range r.Findings {
		if severityRank[r.Findings[i].Severity] < severityRank[level] {
			r.Findings[i] = f
			break
		}
	}
}
func (s riskScan) text(ctx context.Context, b []byte, index, originalLine int) {
	var folding []byte
	if allASCII(b) {
		folding = bytes.ToLower(b)
	}
	for _, sig := range signatures {
		if ctx.Err() != nil {
			return
		}
		// Marker checks skip expensive RE2 passes on ordinary ASCII mail.
		// Non-ASCII still gets the full regex semantics (Unicode case folding).
		if folding != nil && !slices.ContainsFunc(sig.markers, func(word string) bool { return bytes.Contains(folding, []byte(word)) }) {
			continue
		}
		for _, at := range sig.pattern.FindAllIndex(b, 3) {
			line := originalLine
			if line == 0 {
				line = bytes.Count(b[:at[0]], []byte{'\n'}) + 1
			}
			s.add(sig.code, sig.severity, index, line, string(clippedBytes(b[at[0]:at[1]], 512)))
		}
	}
}
func allASCII(b []byte) bool {
	for _, v := range b {
		if v >= 128 {
			return false
		}
	}
	return true
}

func (s riskScan) base64(ctx context.Context, b []byte) {
	var block []byte
	var line, first, total int
	flush := func() {
		if len(block) < 16 || len(block) > maxB64Block || total >= maxB64Total || ctx.Err() != nil {
			block = nil
			return
		}
		data, err := base64.StdEncoding.DecodeString(string(block))
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(string(block))
		}
		block = nil
		if err != nil || len(data)+total > maxB64Total {
			return
		}
		total += len(data)
		if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
			s.text(ctx, data, -1, first)
		}
	}
	for row := range bytes.SplitSeq(b, []byte{'\n'}) {
		line++
		if ctx.Err() != nil || total >= maxB64Total {
			return
		}
		row = bytes.Trim(row, " \t\r")
		if len(row) < 16 || bytes.ContainsAny(row, " \t") {
			flush()
			continue
		}
		m := bodyBlock.FindSubmatch(row)
		if len(m) == 0 {
			flush()
			continue
		}
		if len(block) == 0 {
			first = line
		}
		if len(block) <= maxB64Block {
			n := min(len(m[1]), maxB64Block+1-len(block))
			block = append(block, m[1][:n]...)
		}
		if bytes.HasSuffix(m[1], []byte{'='}) {
			flush()
		}
	}
	flush()
}

var dangerousExt = extSet("exe com scr msi msp pif cpl dll elf so dylib app apk jar class lnk iso img hta sh bash zsh fish ps1 psm1 bat cmd js jse vbs vbe wsf wsh py pyw pl rb")
var documentExt = extSet("pdf doc docx xls xlsx ppt pptx rtf txt csv jpg jpeg png gif tif tiff svg zip tar gz")
var macroExt = extSet("docm dotm xlsm xltm xlam pptm potm ppsm ppam")
var zipExt = extSet("zip docx xlsx pptx docm xlsm pptm")
var inspectTextExt = extSet("sh bash zsh fish ps1 psm1 bat cmd js jse vbs vbe wsf wsh py pyw pl rb hta txt log")

func extSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, v := range strings.Fields(s) {
		m["."+v] = true
	}
	return m
}

func zipMagic(b []byte) bool {
	return bytes.HasPrefix(b, []byte("PK\x03\x04")) || bytes.HasPrefix(b, []byte("PK\x05\x06")) || bytes.HasPrefix(b, []byte("PK\x07\x08"))
}
func imageMagic(b []byte, ext string) bool {
	switch ext {
	case ".pdf":
		return bytes.HasPrefix(b, []byte("%PDF-"))
	case ".png":
		return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n"))
	case ".jpg", ".jpeg":
		return bytes.HasPrefix(b, []byte("\xff\xd8\xff"))
	case ".gif":
		return bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))
	case ".doc", ".xls", ".ppt":
		return bytes.HasPrefix(b, []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"))
	}
	if zipExt[ext] {
		return zipMagic(b)
	}
	return true
}
func executableMagic(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	if bytes.HasPrefix(b, []byte("MZ")) || bytes.HasPrefix(b, []byte("\x7fELF")) {
		return true
	}
	switch binary.BigEndian.Uint32(b) {
	case 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe:
		return true
	}
	return false
}

// Inspect only bounded metadata. No archive member is opened or extracted.
func inspectZIP(b []byte) (macro, encrypted, uninspected bool) {
	if !zipMagic(b) {
		return false, false, false
	}
	start := max(0, len(b)-65557)
	end := -1
	for n := len(b) - 22; n >= start; n-- {
		if bytes.HasPrefix(b[n:], []byte("PK\x05\x06")) && int(binary.LittleEndian.Uint16(b[n+20:])) == len(b)-n-22 {
			end = n
			break
		}
	}
	if end < 0 {
		return false, false, true
	}
	entries := int(binary.LittleEndian.Uint16(b[end+10:]))
	size := int(binary.LittleEndian.Uint32(b[end+12:]))
	pos := int(binary.LittleEndian.Uint32(b[end+16:]))
	if entries > 1024 || size > 512<<10 || pos > end || size > end-pos || binary.LittleEndian.Uint32(b[end+4:]) != 0 {
		return false, false, true
	}
	limit := pos + size
	for j := 0; j < entries; j++ {
		if limit-pos < 46 || !bytes.HasPrefix(b[pos:], []byte("PK\x01\x02")) {
			return macro, encrypted, true
		}
		entry := b[pos:limit]
		nameLen := int(binary.LittleEndian.Uint16(entry[28:]))
		extra := int(binary.LittleEndian.Uint16(entry[30:]))
		comment := int(binary.LittleEndian.Uint16(entry[32:]))
		step := 46 + nameLen + extra + comment
		if step > len(entry) {
			return macro, encrypted, true
		}
		name := filepath.Base(strings.ReplaceAll(string(entry[46:46+nameLen]), "\\", "/"))
		macro = macro || strings.EqualFold(name, "vbaProject.bin")
		encrypted = encrypted || binary.LittleEndian.Uint16(entry[8:])&1 != 0 || binary.LittleEndian.Uint16(entry[10:]) == 99
		pos += step
	}
	return macro, encrypted, false
}

// RAR4's archive password flag indicates encrypted headers. File header flags
// cover password-protected members; compressed data is skipped by length.
func rar4Encrypted(b []byte) bool {
	if !bytes.HasPrefix(b, []byte("Rar!\x1a\x07\x00")) {
		return false
	}
	for i, n := 7, 0; i+7 <= len(b) && n < 1024; n++ {
		kind := b[i+2]
		flags := binary.LittleEndian.Uint16(b[i+3:])
		size := int(binary.LittleEndian.Uint16(b[i+5:]))
		if size < 7 || size > len(b)-i {
			return false
		}
		if (kind == 0x73 && flags&0x80 != 0) || (kind == 0x74 && flags&4 != 0) {
			return true
		}
		extra := uint64(0)
		if flags&0x8000 != 0 && size >= 11 {
			extra = uint64(binary.LittleEndian.Uint32(b[i+7:]))
		}
		if extra > uint64(len(b)-i-size) {
			return false
		}
		i += size + int(extra)
	}
	return false
}
func (s riskScan) attachment(ctx context.Context, i int, a *parsemail.Attachment) {
	if a == nil {
		return
	}
	file := strings.ToLower(filepath.Base(strings.ReplaceAll(a.Filename, "\\", "/")))
	ext := filepath.Ext(file)
	if dangerousExt[ext] {
		s.add("attachment_dangerous_type", "high", i, 0, a.Filename)
		if documentExt[filepath.Ext(strings.TrimSuffix(file, ext))] {
			s.add("attachment_double_extension", "high", i, 0, a.Filename)
		}
	}
	if macroExt[ext] {
		s.add("attachment_macro_document", "high", i, 0, a.Filename)
	}
	if len(a.Content) > 0 && (!imageMagic(a.Content, ext) || (executableMagic(a.Content) && !dangerousExt[ext])) {
		s.add("attachment_type_mismatch", "high", i, 0, a.Filename)
	}
	if macro, encrypted, unknown := inspectZIP(a.Content); macro || encrypted || unknown {
		if macro {
			s.add("attachment_macro_document", "high", i, 0, a.Filename)
		}
		if encrypted {
			s.add("attachment_encrypted_archive", "high", i, 0, a.Filename)
		}
		if unknown {
			s.add("attachment_archive_uninspected", "medium", i, 0, a.Filename)
		}
	}
	seven, uninspected := inspect7z(a.Content)
	if seven || rar4Encrypted(a.Content) || rar5Encrypted(a.Content) {
		s.add("attachment_encrypted_archive", "high", i, 0, a.Filename)
	}
	if uninspected {
		s.add("attachment_archive_uninspected", "medium", i, 0, a.Filename)
	}
	if len(a.Content) > 0 && len(a.Content) <= maxTextAttachment && (inspectTextExt[ext] || parsemail.Previewable(a)) && !bytes.ContainsRune(a.Content, 0) {
		text, _ := parsemail.DecodeText(a.Content, a.Charset)
		s.text(ctx, clippedBytes([]byte(text), maxTextAttachment), i, 0)
	}
}

// RAR5 uses length-prefixed headers and extra records. Inspect only the
// metadata flags, with bounds for counts, declared sizes and integer widths.
func rar5Encrypted(b []byte) bool {
	if !bytes.HasPrefix(b, []byte("Rar!\x1a\x07\x01\x00")) {
		return false
	}
	for pos, blocks := 8, 0; pos+5 <= len(b) && blocks < 1024; blocks++ {
		p := pos + 4
		size, n := binary.Uvarint(b[p:])
		if n <= 0 || n > 3 || size > uint64(len(b)-p-n) {
			return false
		}
		p += n
		end := p + int(size)
		header := b[p:end]
		get := func() (uint64, bool) {
			v, n := binary.Uvarint(header)
			if n <= 0 {
				return 0, false
			}
			header = header[n:]
			return v, true
		}
		kind, ok := get()
		if !ok {
			return false
		}
		flags, ok := get()
		if !ok {
			return false
		}
		if kind == 4 {
			return true
		}
		if kind == 5 {
			return false
		}
		var extra, data uint64
		if flags&1 != 0 {
			if extra, ok = get(); !ok || extra > uint64(len(header)) {
				return false
			}
		}
		if flags&2 != 0 {
			if data, ok = get(); !ok || data > uint64(len(b)-end) {
				return false
			}
		}
		if extra > uint64(len(header)) {
			return false
		}
		if (kind == 2 || kind == 3) && extra > 0 {
			records := header[len(header)-int(extra):]
			for num := 0; len(records) > 0 && num < 256; num++ {
				count, cn := binary.Uvarint(records)
				if cn <= 0 || count == 0 || count > uint64(len(records)-cn) {
					break
				}
				records = records[cn:]
				typ, tn := binary.Uvarint(records[:count])
				if tn > 0 && typ == 1 {
					return true
				}
				records = records[count:]
			}
		}
		pos = end + int(data)
	}
	return false
}

// 7z's plain next-header metadata can identify the AES coder without opening
// archive entries. A compressed next header that hides it is not conclusive.
func inspect7z(b []byte) (encrypted, unknown bool) {
	if !bytes.HasPrefix(b, []byte("7z\xbc\xaf\x27\x1c")) {
		return false, false
	}
	if len(b) < 32 {
		return false, true
	}
	offset := binary.LittleEndian.Uint64(b[12:20])
	size := binary.LittleEndian.Uint64(b[20:28])
	if offset > uint64(len(b)-32) || size == 0 || size > 512<<10 || size > uint64(len(b)-32)-offset {
		return false, true
	}
	h := b[32+offset : 32+offset+size]
	if crc32.ChecksumIEEE(h) != binary.LittleEndian.Uint32(b[28:32]) {
		return false, true
	}
	for i := 1; i+4 < len(h); i++ {
		// The lower nibble in the coder descriptor is the method ID's length.
		if h[i-1]&0x0f == 4 && bytes.Equal(h[i:i+4], []byte{0x06, 0xf1, 0x07, 0x01}) {
			return true, false
		}
	}
	return false, h[0] == 0x17
}
