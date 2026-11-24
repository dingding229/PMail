package translation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const chunkChars = 4500
const bingChunkChars = 1000
const memoryChunkBytes = 500

// chunksText keeps boundaries outside requests so separators lost by a
// provider's trimming are preserved in the reconstructed text.
func chunksText(text, provider string) func(func(string, string) bool) {
	return func(yield func(string, string) bool) {
		r := []rune(text)
		for len(r) > 0 {
			end := chunkEnd(r, provider)
			if end < len(r) && r[end-1] == '\r' && r[end] == '\n' {
				end--
			}
			part := string(r[:end])
			r = r[end:]
			head := strings.TrimRight(part, "\r\n")
			if provider == "mymemory" {
				head = strings.TrimRightFunc(part, unicode.IsSpace)
			}
			if !yield(head, part[len(head):]) {
				return
			}
		}
	}
}

func chunkEnd(r []rune, provider string) int {
	end := min(len(r), chunkChars)
	switch provider {
	case "google_web", "edge_web", "mymemory":
		size := 0
		for i, c := range r[:end] {
			var limit, cost int
			switch provider {
			case "google_web":
				// Both Google URLs carry q. Bound its escaped size separately
				// from the Unicode character count for typical HTTP proxies.
				limit, cost = 7000, 3*utf8.RuneLen(c)
				if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(" -_.~", c) {
					cost = 1
				}
			case "edge_web":
				limit, cost = bingChunkChars, 1
				if c > 0xffff {
					cost = 2
				} // browser length treats non-BMP characters as two units
			case "mymemory":
				limit, cost = memoryChunkBytes, utf8.RuneLen(c)
			}
			size += cost
			if size > limit {
				end = i
				break
			}
		}
	}
	if end == len(r) {
		return end
	}
	if provider == "mymemory" {
		for i := end - 1; i >= 0; i-- {
			if strings.ContainsRune("\n。！？", r[i]) {
				return i + 1
			}
			if strings.ContainsRune(".!?", r[i]) && unicode.IsSpace(r[i+1]) {
				// Keep the spaces alongside the sentence if they fit.
				n := i + 1
				for n < end && unicode.IsSpace(r[n]) {
					n++
				}
				return n
			}
		}
		return end
	}
	paragraphMin := chunkChars / 2
	if provider == "edge_web" {
		paragraphMin = end / 2
	}
	line := 0
	for i := end - 1; i >= 0; i-- {
		if r[i] != '\n' {
			continue
		}
		if line == 0 {
			line = i + 1
		}
		if i > 0 && r[i-1] == '\n' && i+1 >= paragraphMin {
			return i + 1
		}
	}
	if line > 0 {
		return line
	}
	return end
}

type scripts struct{ letters, han, kana, hangul, cyrillic, latin int }

func scriptProfile(text string) (s scripts) {
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		s.letters++
		switch {
		case unicode.Is(unicode.Han, r):
			s.han++
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			s.kana++
		case unicode.Is(unicode.Hangul, r):
			s.hangul++
		case unicode.Is(unicode.Cyrillic, r):
			s.cyrillic++
		case unicode.Is(unicode.Latin, r):
			s.latin++
		}
	}
	return s
}

// noise removes URLs, mail addresses and long codes, which are Latin even in
// a Chinese or Japanese mail and would make it look foreign.
var noise = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|[\w.+-]+@[\w-]+(?:\.[\w-]+)+|\b[\w-]*\d[\w-]{5,}\b`)

// languageUnits counts comparable units: one per Han or kana character, one
// per word for scripts that separate words with spaces. Counting Latin by
// letters made an English word weigh about five Chinese characters.
type languageUnits struct{ total, han, kana, hangul, cyrillic, latin int }

func countUnits(text string) (u languageUnits) {
	text = noise.ReplaceAllString(text, " ")
	var run rune // script of the current word: 'L' Latin, 'C' Cyrillic, 'K' Hangul
	runLen := 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			u.han++
			u.total++
			run = 0
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			u.kana++
			u.total++
			run = 0
		case unicode.Is(unicode.Hangul, r):
			if run != 'K' {
				u.hangul++
				u.total++
			}
			run = 'K'
		case unicode.Is(unicode.Cyrillic, r):
			if run != 'C' {
				u.cyrillic++
				u.total++
			}
			run = 'C'
		case unicode.Is(unicode.Latin, r):
			// A very long token (no spaces, German compounds) counts one
			// unit per 8 letters.
			if run != 'L' || runLen%8 == 0 {
				u.latin++
				u.total++
			}
			if run != 'L' {
				runLen = 0
			}
			runLen++
			run = 'L'
		case unicode.IsLetter(r) || unicode.IsMark(r):
			// other scripts: keep the current run
		default:
			run = 0
		}
	}
	return u
}

// differentScript reports whether text is mostly written in a script other
// than the target language's, so it should be offered for translation.
func differentScript(text, lang string) bool {
	u := countUnits(text)
	if u.total < 4 {
		return false
	}
	matching := u.latin
	switch lang {
	case "zh-CN", "zh-TW":
		// Japanese uses Han too; kana marks it as foreign for Chinese readers.
		if u.kana*4 > u.han+u.kana {
			return true
		}
		matching = u.han
	case "ja":
		matching = u.kana + u.han
	case "ko":
		matching = u.hangul
	case "ru":
		matching = u.cyrillic
	}
	return matching*100 < u.total*35
}

// The existing script heuristic can distinguish non-Latin writing systems,
// not individual Latin languages or the two Chinese variants.
func detectWebLanguage(text string) string {
	s := scriptProfile(text)
	switch {
	case s.kana > 0 && (s.kana+s.han)*2 >= s.letters:
		return "ja"
	case s.hangul > 0 && s.hangul*2 >= s.letters:
		return "ko"
	case s.cyrillic > 0 && s.cyrillic*2 >= s.letters:
		return "ru"
	case s.han > 0 && s.han*2 >= s.letters:
		return "zh-CN"
	default:
		return "en"
	}
}
