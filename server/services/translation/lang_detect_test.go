package translation

import (
	"strings"
	"testing"
)

func TestDifferentScriptWeighsWordsNotLetters(t *testing.T) {
	chineseWithLinks := "您好，您的订单已发货，请点击下方链接查看物流信息。客服邮箱 support@example-shop.com，" +
		"官网 https://www.example-shop.com/orders/track?id=ABCDEF123456&utm_source=email&utm_medium=notice ，" +
		"如有问题请联系 Customer Service Team，感谢您使用 Example Shop Premium Delivery。"
	for _, tc := range []struct {
		name, text, lang string
		want             bool
	}{
		{"Chinese with links and brand names", chineseWithLinks, "zh-CN", false},
		{"Chinese with an English signature", "会议改到明天下午三点，地点不变，请大家准时参加。\n\nBest regards,\nJohn Smith\nSenior Product Manager, Example Inc.", "zh-CN", false},
		{"English mail for Chinese reader", "Your package has shipped and will arrive within three business days. Track it from your account page.", "zh-CN", true},
		{"Japanese mail for Chinese reader", "ご注文ありがとうございます。商品は明日発送される予定です。", "zh-CN", true},
		{"Chinese for English reader", "您的验证码是 123456，十分钟内有效，请勿告诉他人。", "en", true},
		{"English for English reader", "Your package has shipped and will arrive within three business days.", "en", false},
		{"Russian for Russian reader", "Ваш заказ отправлен и прибудет в течение трёх рабочих дней.", "ru", false},
		{"Korean for Korean reader", "주문하신 상품이 내일 발송될 예정입니다. 감사합니다.", "ko", false},
		{"too short to judge", "OK", "zh-CN", false},
		{"long unspaced token", strings.Repeat("abcde", 40), "zh-CN", true},
	} {
		if got := differentScript(tc.text, tc.lang); got != tc.want {
			u := countUnits(tc.text)
			t.Errorf("%s: got %v want %v (%+v)", tc.name, got, tc.want, u)
		}
	}
}
