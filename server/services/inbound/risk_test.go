package inbound

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

func archive(t *testing.T, name string, encrypted bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("inert fixture"))
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if encrypted {
		p := bytes.Index(b, []byte("PK\x01\x02"))
		b[p+8] |= 1
	}
	return b
}
func TestRiskStaticCorpusAndAttachmentIndex(t *testing.T) {
	raw := strings.Join([]string{"curl", "<fixture>", "|", "bash"}, " ")
	disguised := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("# inert fixture\n", 8) + raw))
	email := &parsemail.Email{Text: []byte("Ordinary text\n" + disguised), HTML: []byte("<pre><code>" + strings.Join([]string{"wget", "fixture", "|", "sh"}, " ") + "</code></pre>"), Attachments: []*parsemail.Attachment{
		{Filename: "invoice.pdf.exe", Content: []byte("inert")},
		{Filename: "office.docx", Content: archive(t, "word/vbaProject.bin", false)},
		{Filename: "protected.zip", Content: archive(t, "hello.txt", true)},
		{Filename: "pretend.pdf", Content: []byte("not a PDF")},
		{Filename: "readme.txt", ContentType: "text/plain", Content: []byte("Not executable. Read the documentation.")},
	}}
	a := analyzeShared(context.Background(), email, Envelope{}, nil, Settings{})
	if a == nil || a.Risk.Level != "high" || len(a.HighRiskParts) != 4 {
		t.Fatalf("risk=%+v", a)
	}
	want := map[string]bool{"shell_curl_pipe_sh": false, "shell_wget_pipe_sh": false, "attachment_double_extension": false, "attachment_macro_document": false, "attachment_encrypted_archive": false, "attachment_type_mismatch": false}
	for _, f := range a.Risk.Findings {
		if _, ok := want[f.Code]; ok {
			want[f.Code] = true
		}
		if utf8.RuneCountInString(f.Excerpt) > 200 || strings.ContainsAny(f.Excerpt, "<>\r\n") {
			t.Fatalf("unsafe excerpt %q", f.Excerpt)
		}
		if f.Location == "attachment" && (f.AttachmentIndex == nil || *f.AttachmentIndex == 4) {
			t.Fatalf("incorrect attachment %+v", f)
		}
	}
	for code, hit := range want {
		if !hit {
			t.Errorf("missing detection %s", code)
		}
	}
	clean := analyzeShared(context.Background(), &parsemail.Email{Text: []byte("A safe note about curl and the shell, without a pipeline.")}, Envelope{}, nil, Settings{})
	if clean.Risk.Level != "none" {
		t.Fatalf("false positive: %+v", clean.Risk)
	}
}

func TestAnalysisCanceledBeforeWork(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()
	shared, perOwner := AnalyzeDelivery(ctx, &parsemail.Email{Subject: "OTP: 234567"}, Envelope{}, nil, []int{1})
	if shared != nil || perOwner != nil {
		t.Fatal("expired budget must leave message unanalysed")
	}
}
