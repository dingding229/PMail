package inbound

import (
	"mime"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

var verificationLineEndings = [...]struct{ name, separator string }{
	{"LF", "\n"}, {"CRLF", "\r\n"}, {"CR", "\r"},
}

func TestVerificationCorpus(t *testing.T) {
	positive := []struct{ subject, body, want string }{
		{"Apple ID verification code: 480161", "", "480161"},
		{"Your Apple verification code is 615904", "Never share it.", "615904"},
		{"Apple 登录通知", "您的验证码为 753192，请勿向他人透露。", "753192"},
		{"Account security", "Use this one-time code: 781029", "781029"},
		{"Google verification", "Your Google verification code is 349581.", "349581"},
		{"Google 登录验证码：612877", "", "612877"},
		{"Confirm recovery", "Verification code: 618822\n> OTP: 999991", "618822"},
		{"Account check", "OTP: 5703", "5703"},
		{"微信安全提醒", "登录验证码：421965", "421965"},
		{"微信确认码 778345", "请核实登录环境", "778345"},
		{"微信支付", "请使用动态码 912654 完成确认", "912654"},
		{"微信设备验证", "验证码是【138975】。", "138975"},
		{"支付宝安全验证", "您的校验码是 892754。", "892754"},
		{"支付宝付款确认码 624431", "", "624431"},
		{"支付宝身份校验", "安全码：908435。请勿泄露", "908435"},
		{"账户登录", "本次验证码为 102366。若非本人操作请忽略。", "102366"},
		{"示例银行交易验证", "请填写动态码 480573。交易金额 ¥7985。", "480573"},
		{"Example Bank security", "One-time passcode: 637928", "637928"},
		{"安全码 519348", "示例银行提醒。订单号 115194", "519348"},
		{"Sign in to Example Bank", "872301 is your passcode", "872301"},
		{"GitHub sign-in verification", "Your verification code is 946311", "946311"},
		{"GitHub 2FA: MY57X9", "", "MY57X9"},
		{"GitHub 登录", "请输入确认码 HK3J8P", "HK3J8P"},
		{"Confirm a new device", "Device code: 8FJ24KD", "8FJ24KD"},
		{"Your one-time password: 31005992", "", "31005992"},
		{"OTP = AB4Z", "", "AB4Z"},
		{"邮箱校验码：7RE3Z", "", "7RE3Z"},
		{"身份验证", "校验码 R7U29PQX", "R7U29PQX"},
		{"账号验证", "安全码: X2NZ7U", "X2NZ7U"},
		{"Sign-in verification", "Passcode: 613688. Valid for 10 minutes.", "613688"},
		{"One time code: 420813", "Contact (415) 555-0101 for help.", "420813"},
		{"Confirm your code", "Use OTP 793516 to sign in.", "793516"},
		{"远程登录", "验证码：261948。\n---- 原始邮件 ----\n验证码：894321", "261948"},
		{"账户验证", "确认码为 967210。确认无误后登录。", "967210"},
		{"Login verification", "Code is 540019\nOn Monday, Example wrote:\nCode: 884122", "540019"},
		{"Subscription confirmation", "verification code = 643972\nURL: https://example.test/643972", "643972"},
		{"新的设备登录", "【安全码】 793805", "793805"},
		{"校园邮箱", "请输入动态码 NZ7E4A", "NZ7E4A"},
		{"Reset account", "Enter this OTP: f3k8z2", "f3k8z2"},
		{"预约身份验证", "验证码：048159，五分钟内有效。", "048159"},
		{"Security code: 348791", "For your records, invoice ID 75201", "348791"},
		{"付款确认", "确认码 003538", "003538"},
		{"", "您的登录验证码是：\n\n  662801\n", "662801"},
		{"", "Your verification code:\n\n482913\n\nThis code expires in 10 minutes.", "482913"},
		{"Your sign-in code", "Use 7G4K2Q to sign in.", "7G4K2Q"},
		{"", "动态密码：583920", "583920"},
		{"", "验证码 0 0 1 2 3 4 无效。\n真正的验证码：521603", "521603"},
		{"包裹通知", "取件码：\n\t85X31", "85X31"},
		{"", "短信验证码为：\n\n238683", "238683"},
		{"账号", "登录码：\n 398112", "398112"},
		{"", "授权码 391422", "391422"},
		{"", "Security code:\n 481728", "481728"},
		{"Login code", "Enter 61BF21 to continue.", "61BF21"},
		{"Confirmation code", "  748426 ", "748426"},
		{"", "PIN:\n\n791043", "791043"},
		{"登录验证码", "  003521\n", "003521"},
		{"", "验证码 110892 无效\n动态密码：416127", "416127"},
	}
	negative := []struct{ subject, body string }{
		{"Verification code expires", "Valid for 1200 seconds."},
		{"验证码", "时间为 2026-11-23 14:35:04"},
		{"Code: 2026", ""}, {"验证码：2025", ""},
		{"Verification code reminder", "Contact 415-555-0132"},
		{"OTP assistance", "phone: 13800138000"},
		{"验证码通知", "校验码 https://example.test/123456"},
		{"Passcode assistance", "Visit https://example.test/reset/393984"},
		{"付款验证", "验证码服务收费 ¥554321"},
		{"Bank passcode notice", "Passcode fee: $459428"},
		{"订单确认", "验证码订单号：594239"},
		{"Reset code notice", "Order number: GH12XT"},
		{"Code: XYQTEST", ""},
		{"Code 123456789", ""},
		{"Re: support", "> Apple verification code is 091927"},
		{"账户协助", "---原始邮件---\n验证码：932171"},
		{"Fwd: hello", "On Monday, Example wrote:\nCode: 472391"},
		{"Postal code: 50423", ""},
		{"Status code: 4031", ""},
		{"Product code: 451122", ""},
		{"验证码说明", "验证码有效期还剩 9000 秒钟"},
		{"Tracking code: RF58ZT", ""},
		{"订单详情", "订单号 883917，金额 ¥1228，客服电话 400-123-4567"},
		{"会议邀请", "会议时间 14:35，房间号 5199"},
		{"Invoice", "发票号 448812\nReference code: INV-2026"},
	}
	for _, ending := range verificationLineEndings {
		t.Run(ending.name, func(t *testing.T) {
			withLineEnding := func(body string) []byte { return []byte(strings.ReplaceAll(body, "\n", ending.separator)) }
			pass, rejected := 0, 0
			for _, tc := range positive {
				got := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: withLineEnding(tc.body)})
				if got == nil || got.Code != tc.want {
					t.Errorf("%q / %q: got %+v, want %s", tc.subject, tc.body, got, tc.want)
					continue
				}
				if ending.separator != "\n" {
					lf := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: []byte(tc.body)})
					if lf == nil || *got != *lf {
						t.Errorf("%q / %q: got %+v, differs from LF result %+v", tc.subject, tc.body, got, lf)
						continue
					}
				}
				pass++
			}
			for _, tc := range negative {
				got := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: withLineEnding(tc.body)})
				if got != nil {
					t.Errorf("negative %q / %q: got %+v", tc.subject, tc.body, got)
				} else {
					rejected++
				}
			}
			t.Logf("corpus: positives %d/%d, negatives %d/%d", pass, len(positive), rejected, len(negative))
		})
	}
}

func TestVerificationSubjectFallbackLineEndings(t *testing.T) {
	for _, tc := range []struct{ name, subject, body, want string }{
		{"bare code", "仅主题关键词验证码", "123987\n", "123987"},
		{"possessive Chinese prefix", "登录验证码", "您的代码是 234567\n", "234567"},
		{"Chinese prefix with colon", "登录验证码", "代码：234567\n", "234567"},
		{"English prefix", "登录验证码", "Your code is 234567\n", "234567"},
		{"Chinese prefix needs strong subject", "账号通知", "代码：234567\n", ""},
		{"reject generic error code", "验证码", "错误代码 500123，请联系客服\n", ""},
		{"reject generic product code", "验证码", "产品代码：A12345\n", ""},
		{"reject multiple fallback candidates", "验证码", "您的代码是 234567\n代码：345678\n", ""},
		{"ignore quoted old code", "验证码", "代码：234567\nOn Monday, Example wrote:\n代码：345678\n", "234567"},
	} {
		for _, ending := range verificationLineEndings {
			t.Run(tc.name+"/"+ending.name, func(t *testing.T) {
				body := strings.ReplaceAll(tc.body, "\n", ending.separator)
				got := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: []byte(body)})
				if tc.want == "" {
					if got != nil {
						t.Fatalf("got %+v, want no code", got)
					}
					return
				}
				if got == nil || got.Code != tc.want || got.Source != "body" {
					t.Fatalf("got %+v, want %s from body", got, tc.want)
				}
			})
		}
	}
	// The English form also matches the general detector; exercise the fallback directly.
	if hit, ok := subjectOnlyCode("登录验证码", "Your code is 234567\n"); !ok || hit.value != "234567" {
		t.Fatalf("English fallback got %+v, %t", hit, ok)
	}
}

func TestVerificationSubjectFallbackFromSMTPMessage(t *testing.T) {
	const senderAndRecipient = "From: sender@example.test\r\nTo: user@example.test\r\n"
	const plainText = "\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n"
	for _, tc := range []struct{ subject, body, want string }{
		{"仅主题关键词验证码", "123987", "123987"},
		{"登录验证码", "您的代码是 234567", "234567"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			wire := senderAndRecipient + "Subject: " + mime.QEncoding.Encode("utf-8", tc.subject) + plainText + tc.body + "\r\n"
			email, err := parsemail.ParseLimited([]string{"user@example.test"}, strings.NewReader(wire), len(wire))
			if err != nil {
				t.Fatal(err)
			}
			if got := recognizeCode(email); got == nil || got.Code != tc.want || got.Source != "body" {
				t.Fatalf("got %+v, want %s from parsed body", got, tc.want)
			}
		})
	}
}

func TestVerificationSubjectFallbackNormalizesHTMLText(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"HTML with CRLF", "<pre>您的代码是 234567\r\n请勿分享此码。</pre>"},
		{"HTML with encoded CR", "<pre>您的代码是 234567&#13;请勿分享此码。</pre>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := recognizeCode(&parsemail.Email{Subject: "登录验证码", HTML: []byte(tc.body), TextFromHTML: true})
			if got == nil || got.Code != "234567" || got.Source != "body" {
				t.Fatalf("got %+v, want 234567 from HTML body", got)
			}
		})
	}
}

func TestPickupKind(t *testing.T) {
	code := recognizeCode(&parsemail.Email{Text: []byte("取件码：\n614930")})
	if code == nil || code.Kind != "pickup" {
		t.Fatalf("pickup: %+v", code)
	}
}

func TestVerificationSourcesConfidenceAndQuotedHTML(t *testing.T) {
	seed := &parsemail.Email{Subject: "OTP: 748593", HTML: []byte(`<blockquote>OTP: 667382</blockquote><p>Your passcode is 793859.</p>`), TextFromHTML: true}
	a := recognizeCode(seed)
	if a == nil || a.Source != "subject" || a.Code != "748593" || a.Confidence >= .95 {
		t.Fatalf("ambiguous: %+v", a)
	}
	seed.Subject = "Hello"
	b := recognizeCode(seed)
	if b == nil || b.Source != "body" || b.Code != "793859" {
		t.Fatalf("quoted HTML: %+v", b)
	}
	seed.HTML = []byte(strings.Repeat(" ", 4*codeBodyLimit+1) + "<p>OTP 395094</p>")
	if got := recognizeCode(seed); got != nil {
		t.Fatalf("body cutoff: %+v", got)
	}
}

func TestVerificationCandidateRanking(t *testing.T) {
	for _, tc := range []struct{ name, subject, body, want, source string }{
		{"TB18 subject prefix", "TB18 验证码样例", "登录验证码：\n123456", "123456", "body"},
		{"ABC1 subject prefix", "ABC1 验证码", "登录验证码：\n123456", "123456", "body"},
		{"numeric body with subject label only", "ABC1 验证码", "123456", "123456", "body"},
		{"numeric subject code without body", "Your code is 482913", "", "482913", "subject"},
		{"numeric subject code with instructions", "Your code is 482913", "Do not share this code.", "482913", "subject"},
		{"mixed subject code with instructions", "验证码 A1B2C3", "请勿透露给他人。", "A1B2C3", "subject"},
		{"year is not a code", "订单 2024 验证码", "", "", ""},
		{"order identifier is not a code", "Order #A12345 verification code", "Your code: 778899", "778899", "body"},
		{"body after keyword outranks numeric subject prefix", "876543 验证码", "登录验证码是本次账户登录时需要输入的动态凭证 123456", "123456", "body"},
		{"mixed subject code gets less weight with numeric body code", "验证码：A1B2C3", "Your code: 778899", "778899", "body"},
		{"mixed body code gets less weight with numeric body code", "", "验证码：A1B2C3。\n真正的验证码为 123456", "123456", "body"},
		{"subject code before label remains eligible", "872301 is your passcode", "", "872301", "subject"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := recognizeCode(&parsemail.Email{Subject: tc.subject, Text: []byte(tc.body)})
			if tc.want == "" {
				if got != nil {
					t.Fatalf("got %+v, want no code", got)
				}
				return
			}
			if got == nil || got.Code != tc.want || got.Source != tc.source {
				t.Fatalf("got %+v, want %s from %s", got, tc.want, tc.source)
			}
		})
	}
}
