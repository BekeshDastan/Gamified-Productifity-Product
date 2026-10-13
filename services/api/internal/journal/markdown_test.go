package journal

import (
	"strings"
	"testing"
)

func TestMarkdownEscapesUntrustedContent(t *testing.T) {
	if out := RenderMarkdown("[**label**](https://example.test/**literal**)"); !strings.Contains(out, `href="https://example.test/**literal**"`) || !strings.Contains(out, "<strong>label</strong>") {
		t.Fatal(out)
	}
	for _, body := range []string{`<script>alert(1)</script>`, `<img src=x onerror=alert(1)>`, `[click](javascript:alert)`, `[click](data:text/html)`, `[click](https://example.test/"onclick="alert)`, "```\n<script>bad</script>\n```", "[**x**](https://example.test)"} {
		out := RenderMarkdown(body)
		if strings.Contains(out, "<script") || strings.Contains(out, "<img") || strings.Contains(out, `href="javascript:`) || strings.Contains(out, `href="data:`) || strings.Contains(out, `"onclick="`) {
			t.Fatal(body, out)
		}
	}
	if out := RenderMarkdown("# Заголовок\n**Текст** и *курсив*\n- список\n[ссылка](https://example.test)"); !strings.Contains(out, "<strong>Текст</strong>") || !strings.Contains(out, `href="https://example.test"`) {
		t.Fatal(out)
	}
}
