package llmservice

import (
	"strings"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com/chat/completions": "https://api.deepseek.com/",
		"https://api.deepseek.com/responses":        "https://api.deepseek.com/",
		"https://api.deepseek.com":                  "https://api.deepseek.com/",
		"https://api.deepseek.com/":                 "https://api.deepseek.com/",
		"  https://api.openai.com/v1/completions  ":    "https://api.openai.com/v1/",
		"": "",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanOutput(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		source string
		want   string
	}{
		{"原样", "你好", "Hello", "你好"},
		{"开场白", "好的，以下是译文：\n你好", "Hello", "你好"},
		{"边界标记", "<待翻译文本>\n你好\n</待翻译文本>", "Hello", "你好"},
		{"代码块", "```\n你好\n```", "Hello", "你好"},
		{"带语言标注的代码块", "```text\n你好\n```", "Hello", "你好"},
		{"多行保留", "第一行\n第二行", "line1\nline2", "第一行\n第二行"},
		{"正文含译文二字不剥", "这是一份译文质量报告", "A translation quality report", "这是一份译文质量报告"},

		// 数据损坏防线：单元格里合法的尖括号内容不能被当成边界标记删掉。
		{"合法尖括号保留", "在此处插入 <TEXT>", "Insert <TEXT> here", "在此处插入 <TEXT>"},
		{"HTML 片段保留", "<b>粗体</b>", "<b>bold</b>", "<b>粗体</b>"},

		// 原文自带"标签："前缀时，同形的译文前缀是正文而非开场白。
		{"原文自带标签前缀", "译文：你好", "Translation: hello", "译文：你好"},
		// 剥完会变空时不剥，避免把整格内容清掉。
		{"剥完为空则不剥", "以下是译文：", "Here is the translation:", "以下是译文："},
		{"原文无标签但剥完为空，仍不剥", "翻译结果：", "Result", "翻译结果："},
	}
	for _, tc := range cases {
		if got := cleanOutput(tc.raw, tc.source); got != tc.want {
			t.Errorf("%s: cleanOutput(%q, %q) = %q, want %q", tc.name, tc.raw, tc.source, got, tc.want)
		}
	}
}

func TestDetectUntranslated(t *testing.T) {
	s := &LLMService{}

	cases := []struct {
		name       string
		source     string
		translated string
		want       string
	}{
		{"正常翻译", "Hello world", "你好，世界", outcomeSuccess},
		{"原样回显", "Hello world", "Hello world", outcomeEchoedSource},
		{"仅空白差异也算回显", "Hello world", "Helloworld", outcomeEchoedSource},
		// 回显一律可疑，不猜目标语言；重试提示允许模型坚持原样，最终回退原文。
		{"中文回显也算可疑", "季度销售额汇总", "季度销售额汇总", outcomeEchoedSource},
		{"日文回显算可疑", "売上高の四半期集計です", "売上高の四半期集計です", outcomeEchoedSource},
		{"夹带原文副本", "Quarterly sales summary", "季度销售汇总（Quarterly sales summary）", outcomeRepeatedSource},
		{"短原文不按夹带处理", "N/A ok", "不适用 N/A ok", outcomeSuccess},

		// 关键回归：这些译文含 reasoning 元话语词，但都是完全正确的译文，
		// 绝不能被判成失败——否则正确结果会被丢弃、单元格永久保留原文。
		{"译文含「用户想要」", "The user wants to export the report", "用户想要导出报告", outcomeSuccess},
		{"译文含「用户要求」", "The user asked for a refund", "用户要求退款", outcomeSuccess},
		{"译文含「我需要翻译」", "I need to translate this document", "我需要翻译这份文档", outcomeSuccess},
		{"译文含「原文是」", "The source text is in English", "原文是英文", outcomeSuccess},
	}
	for _, tc := range cases {
		if got := s.detectUntranslated(tc.source, tc.translated); got != tc.want {
			t.Errorf("%s: detectUntranslated(%q, %q) = %q, want %q", tc.name, tc.source, tc.translated, got, tc.want)
		}
	}
}

// 回显原文的重试提示不得要求"输出必须与原文不同"：那会把 Microsoft Office
// 这类专有名词逼成错译，且错译能通过全部检测直接写盘。
func TestEchoRetryHintDoesNotForceDifference(t *testing.T) {
	hint := retryHint(outcomeEchoedSource)
	for _, forbidden := range []string{"必须是目标语言的译文且与原文不同", "与原文不同"} {
		if strings.Contains(hint, forbidden) {
			t.Errorf("回显重试提示不应包含 %q，会逼出专有名词错译：%s", forbidden, hint)
		}
	}
	if !strings.Contains(hint, "专有名词") {
		t.Errorf("回显重试提示应明确允许专有名词保持原样：%s", hint)
	}
}

func TestRetryHintPerOutcome(t *testing.T) {
	if retryHint(outcomeEmptyOutput) == retryHint(outcomeEchoedSource) {
		t.Error("空输出应有独立的重试提示，不能复用「与原文一致」的措辞")
	}
	if retryHint(outcomeRepeatedSource) == retryHint(outcomeEchoedSource) {
		t.Error("夹带原文应有独立的重试提示")
	}
}

func TestReadsLikeReasoning(t *testing.T) {
	if !readsLikeReasoning("用户要求把这句译成中文，答案是你好") {
		t.Error("含元话语的思考文本应被识别")
	}
	if readsLikeReasoning("你好，世界") {
		t.Error("普通译文不应被识别为思考过程")
	}
}

func TestNeedsTranslation(t *testing.T) {
	cases := map[string]bool{
		"2024":      false,
		"—":         false,
		"12.5%":     false,
		"( )":       false,
		"N/A":       true,
		"合计":        true,
		"Total 100": true,
	}
	for in, want := range cases {
		if got := needsTranslation(in); got != want {
			t.Errorf("needsTranslation(%q) = %v, want %v", in, got, want)
		}
	}
}
