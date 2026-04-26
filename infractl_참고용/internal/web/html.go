// Package web
// File: html.go
// Description: HTML을 Markdown으로 변환하는 경량 변환기
// Responsibility: golang.org/x/net/html 토크나이저 기반으로 불필요 태그 제거 후 Markdown 생성

package web

import (
	"strings"

	"golang.org/x/net/html"
)

const defaultMaxMarkdownLen = 100000

// skipTags는 내용을 통째로 무시할 태그 집합이다.
var skipTags = map[string]bool{
	"script": true, "style": true, "nav": true, "header": true,
	"footer": true, "aside": true, "noscript": true, "svg": true,
	"form": true, "iframe": true,
}

// voidElements는 닫는 태그가 없는 HTML void 요소 집합이다.
// skipDepth 추적 시 이 요소들은 카운터를 증가시키지 않아야 한다.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// blockTags는 앞뒤 개행이 필요한 블록 태그이다.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"main": true, "blockquote": true, "figure": true,
}

// HTMLToMarkdown은 HTML 바이트를 Markdown 텍스트로 변환한다.
// maxLen이 0이면 defaultMaxMarkdownLen을 사용한다.
func HTMLToMarkdown(htmlBytes []byte, maxLen int) string {
	if maxLen <= 0 {
		maxLen = defaultMaxMarkdownLen
	}

	tokenizer := html.NewTokenizer(strings.NewReader(string(htmlBytes)))
	var sb strings.Builder
	skipDepth := 0 // skipTags 안에 있는 깊이
	var hrefStack []string
	inPre := false

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}

		token := tokenizer.Token()

		switch tt {
		case html.SelfClosingTagToken:
			// Self-closing 태그는 end tag가 없으므로 skipDepth를 변경하지 않는다.
			if skipDepth > 0 {
				continue
			}
			// skip 대상이 아닌 self-closing 태그는 아래 StartTagToken과 동일하게 처리한다.
			tag := token.Data
			switch tag {
			case "br":
				sb.WriteString("\n")
			}
			continue

		case html.StartTagToken:
			tag := token.Data
			if skipDepth > 0 {
				// void 요소는 닫는 태그가 없으므로 skipDepth를 올리지 않는다.
				if !voidElements[tag] {
					skipDepth++
				}
				continue
			}
			if skipTags[tag] {
				skipDepth++
				continue
			}
			switch tag {
			case "h1":
				sb.WriteString("\n# ")
			case "h2":
				sb.WriteString("\n## ")
			case "h3":
				sb.WriteString("\n### ")
			case "h4", "h5", "h6":
				sb.WriteString("\n#### ")
			case "li":
				sb.WriteString("\n- ")
			case "br":
				sb.WriteString("\n")
			case "code":
				if !inPre {
					sb.WriteString("`")
				}
			case "pre":
				inPre = true
				sb.WriteString("\n```\n")
			case "a":
				href := attrVal(token.Attr, "href")
				hrefStack = append(hrefStack, href)
				sb.WriteString("[")
			case "strong", "b":
				sb.WriteString("**")
			case "em", "i":
				sb.WriteString("*")
			default:
				if blockTags[tag] {
					sb.WriteString("\n")
				}
			}

		case html.EndTagToken:
			tag := token.Data
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			switch tag {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				sb.WriteString("\n")
			case "code":
				if !inPre {
					sb.WriteString("`")
				}
			case "pre":
				inPre = false
				sb.WriteString("\n```\n")
			case "a":
				href := ""
				if len(hrefStack) > 0 {
					href = hrefStack[len(hrefStack)-1]
					hrefStack = hrefStack[:len(hrefStack)-1]
				}
				if href != "" && !strings.HasPrefix(href, "javascript:") {
					sb.WriteString("](")
					sb.WriteString(href)
					sb.WriteString(")")
				} else {
					sb.WriteString("]")
				}
			case "strong", "b":
				sb.WriteString("**")
			case "em", "i":
				sb.WriteString("*")
			default:
				if blockTags[tag] {
					sb.WriteString("\n")
				}
			}

		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			text := token.Data
			if !inPre {
				// 연속 공백/개행 정리
				text = strings.Join(strings.Fields(text), " ")
				if text == "" {
					continue
				}
				text = " " + text
			}
			sb.WriteString(text)
		}

		if sb.Len() >= maxLen {
			break
		}
	}

	result := strings.TrimSpace(normalizeNewlines(sb.String()))
	if len(result) > maxLen {
		result = result[:maxLen] + "\n...(truncated)"
	}
	return result
}

// attrVal은 HTML 속성 목록에서 특정 키의 값을 반환한다.
func attrVal(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// normalizeNewlines는 3개 이상 연속 개행을 2개로 줄인다.
func normalizeNewlines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
