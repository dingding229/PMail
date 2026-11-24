package inbound

import (
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

func TestVerificationMultilingualLabels(t *testing.T) {
	for _, tc := range []struct{ subject, body, want string }{
		{"Inicio de sesión", "Tu código de verificación es 482913.", "482913"},
		{"Acesso à conta", "Seu código de verificação: 551204", "551204"},
		{"Connexion", "Votre code de vérification est 730918", "730918"},
		{"Accesso", "Il tuo codice di verifica è 384021", "384021"},
		{"Anmeldung", "Ihr Bestätigungscode lautet 629184", "629184"},
		{"Aanmelden", "Je verificatiecode is 902345", "902345"},
		{"Logowanie", "Twój kod weryfikacyjny: 118273", "118273"},
		{"Přihlášení", "Váš ověřovací kód je 662801", "662801"},
		{"Вход в аккаунт", "Ваш код подтверждения: 745120", "745120"},
		{"Вхід", "Код підтвердження 309182", "309182"},
		{"Σύνδεση", "Ο κωδικός επαλήθευσης είναι 552019", "552019"},
		{"Giriş", "Doğrulama kodunuz: 671203", "671203"},
		{"Masuk", "Kode verifikasi Anda adalah 450912", "450912"},
		{"Đăng nhập", "Mã xác minh của bạn là 781029", "781029"},
		{"تسجيل الدخول", "رمز التحقق الخاص بك هو 615904", "615904"},
		{"ورود", "کد تأیید شما: 349581", "349581"},
		{"התחברות", "קוד האימות שלך: 612877", "612877"},
		{"लॉगिन", "आपका सत्यापन कोड 892754 है", "892754"},
		{"เข้าสู่ระบบ", "รหัสยืนยันของคุณคือ 908435", "908435"},
		{"ログイン", "確認コード：102366", "102366"},
		{"Kirjautuminen", "Vahvistuskoodi: 637928", "637928"},
		{"Bejelentkezés", "Az ellenőrző kód: 946311", "946311"},
	} {
		got := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: []byte(tc.body)})
		if got == nil || got.Code != tc.want {
			t.Errorf("%q / %q: got %+v, want %s", tc.subject, tc.body, got, tc.want)
		}
	}
}

func TestVerificationLabelWordBoundary(t *testing.T) {
	// "кодекс" (code of law) and "kodak" contain a label but are other words.
	for _, body := range []string{"Статья кодекса 482913 утратила силу", "Kodak 482913 film"} {
		if got := recognizeCode(&parsemail.Email{Subject: "News", Text: []byte(body)}); got != nil {
			t.Errorf("%q: got %+v", body, got)
		}
	}
}

func TestVerificationStructuralFallback(t *testing.T) {
	const unknown = "Ndiyo tafadhali tumia nambari hii" // Swahili, no known label
	for _, tc := range []struct {
		name, from, subject, text, html, want string
	}{
		{"own line and in subject", "", "【842884】" + unknown, unknown + "\n\n▼\n842884\n", "", "842884"},
		{"bracketed subject, inline body", "", "[551204] " + unknown, unknown + " 551204 sasa.", "", "551204"},
		{"large type from automated sender", "no-reply@accounts.example.test", unknown, "", `<p>` + unknown + `</p><div style="font-size:32px;letter-spacing:6px">730918</div>`, "730918"},
		{"own line from automated sender", "noreply@example.test", unknown, unknown + "\n\n  384021\n\n" + unknown, "", "384021"},
		{"bold mixed code on own line", "security@example.test", unknown, "", `<p>` + unknown + `</p><p><b>X7K2Q9</b></p>`, "X7K2Q9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &parsemail.Email{Subject: tc.subject, Text: []byte(tc.text), HTML: []byte(tc.html), From: &parsemail.User{EmailAddress: tc.from}}
			if tc.text == "" {
				e.TextFromHTML = true
			}
			got := recognizeCode(e)
			if got == nil || got.Code != tc.want {
				t.Fatalf("got %+v, want %s", got, tc.want)
			}
		})
	}
}

func TestVerificationStructuralNegatives(t *testing.T) {
	for _, tc := range []struct{ name, from, subject, text, html string }{
		{"personal mail with a lone number", "friend@example.test", "Hi", "See you at\n\n4521\n", ""},
		{"order number under its label", "noreply@shop.example.test", "Your order", "Order number\n558213\nThank you", ""},
		{"Chinese order number", "noreply@shop.example.test", "订单通知", "订单编号：\n558213", ""},
		{"two standalone numbers", "noreply@example.test", "Report", "\n558213\n\n771902\n", ""},
		{"short lone number from automated sender", "noreply@example.test", "Weekly", "Steps\n\n8812\n", ""},
		{"bold total amount", "noreply@shop.example.test", "Receipt", "", `<p>Total</p><p><b>129900</b></p>`},
		{"capital word", "noreply@example.test", "Hello", "\nWELCOME\n", ""},
		{"year on its own line", "noreply@example.test", "2026", "\n2026\n", ""},
		{"quoted code", "friend@example.test", "Re: 482913", "", `<blockquote><b>482913</b></blockquote>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &parsemail.Email{Subject: tc.subject, Text: []byte(tc.text), HTML: []byte(tc.html), From: &parsemail.User{EmailAddress: tc.from}}
			if tc.text == "" {
				e.TextFromHTML = true
			}
			if got := recognizeCode(e); got != nil {
				t.Fatalf("got %+v, want none", got)
			}
		})
	}
}
