package journal

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

var markdownLink = regexp.MustCompile(`\[([^\]\n]+)\]\(([^\s()]+)\)`)
var markdownStrong = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
var markdownEmphasis = regexp.MustCompile(`\*([^*\n]+)\*`)

// RenderMarkdown deliberately supports text, headings, lists, emphasis, fenced
// code and HTTP(S) links. Raw HTML, images and embedded content remain text.
func RenderMarkdown(body string) string {
	inline := func(text string) string {
		plain := func(value string) string {
			escaped := html.EscapeString(value)
			escaped = markdownStrong.ReplaceAllString(escaped, "<strong>$1</strong>")
			return markdownEmphasis.ReplaceAllString(escaped, "<em>$1</em>")
		}
		var out strings.Builder
		position := 0
		for _, match := range markdownLink.FindAllStringSubmatchIndex(text, -1) {
			out.WriteString(plain(text[position:match[0]]))
			target := text[match[4]:match[5]]
			u, e := url.Parse(target)
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
				out.WriteString(plain(text[match[0]:match[1]]))
			} else {
				out.WriteString(`<a href="` + html.EscapeString(target) + `" rel="noopener noreferrer" target="_blank">` + plain(text[match[2]:match[3]]) + `</a>`)
			}
			position = match[1]
		}
		out.WriteString(plain(text[position:]))
		return out.String()
	}
	var out strings.Builder
	code := false
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "```") {
			if code {
				out.WriteString("</code></pre>")
			} else {
				out.WriteString("<pre><code>")
			}
			code = !code
			continue
		}
		if code {
			out.WriteString(html.EscapeString(line) + "\n")
			continue
		}
		switch {
		case strings.HasPrefix(line, "### "):
			out.WriteString("<h4>" + inline(line[4:]) + "</h4>")
		case strings.HasPrefix(line, "## "):
			out.WriteString("<h3>" + inline(line[3:]) + "</h3>")
		case strings.HasPrefix(line, "# "):
			out.WriteString("<h2>" + inline(line[2:]) + "</h2>")
		case strings.HasPrefix(line, "- "):
			out.WriteString("<ul><li>" + inline(line[2:]) + "</li></ul>")
		case strings.TrimSpace(line) == "":
			out.WriteString("<br>")
		default:
			out.WriteString("<p>" + inline(line) + "</p>")
		}
	}
	if code {
		out.WriteString("</code></pre>")
	}
	return out.String()
}
