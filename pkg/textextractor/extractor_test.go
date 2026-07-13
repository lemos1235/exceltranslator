package textextractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldExtractXML_PPTX(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"ppt/slides/slide1.xml", true},
		{"ppt/slides/slide8.xml", true},
		{"ppt/notesSlides/notesSlide1.xml", true},
		{"ppt/slideLayouts/slideLayout1.xml", true},
		{"ppt/slides/_rels/slide1.xml.rels", false},
		{"ppt/slideMasters/slideMaster1.xml", false},
		{"ppt/theme/theme1.xml", false},
		{"xl/drawings/drawing1.xml", true},
		{"word/document.xml", true},
	}
	for _, tc := range cases {
		if got := ShouldExtractXML(tc.path); got != tc.want {
			t.Errorf("ShouldExtractXML(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestExtract_PPTXSlide(t *testing.T) {
	// Prefer demo file if present; fall back to inline sample.
	demoPath := filepath.Join("..", "..", "demo", "demo1", "ppt", "slides", "slide2.xml")
	content, err := os.ReadFile(demoPath)
	if err != nil {
		content = []byte(`<?xml version="1.0"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
 xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
<p:cSld><p:spTree>
<p:sp><p:txBody><a:p><a:r><a:t>次期基幹システム提案</a:t></a:r></a:p>
<a:p><a:r><a:t>背景・目的</a:t></a:r></a:p>
<a:p><a:r><a:t xml:space="preserve">形状テキスト </a:t></a:r></a:p>
<a:p><a:fld type="slidenum"><a:t>2</a:t></a:fld></a:p>
<a:tabLst><a:tab pos="0" algn="l"/></a:tabLst>
</p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`)
	}

	e := NewExtractor(ExtractorConfig{CJKOnly: false})
	_, items, err := e.Extract(string(content), "ppt/slides/slide2.xml")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected extracted items from PPTX slide")
	}

	var joined strings.Builder
	for _, item := range items {
		joined.WriteString(item.Text)
		// Page numbers / pure symbols should not be extracted.
		if item.Text == "2" || item.Text == "PAGE" {
			// PAGE may still pass IsValidTextContent; pure "2" must not.
			if item.Text == "2" {
				t.Errorf("page number %q should be filtered", item.Text)
			}
		}
	}
	text := joined.String()
	if !strings.Contains(text, "次期") && !strings.Contains(text, "背景") && !strings.Contains(text, "形状") {
		// Demo slide2 has 背景・目的; inline sample has both.
		// Accept either demo content or fallback sample.
		found := false
		for _, item := range items {
			if strings.Contains(item.Text, "基幹") || strings.Contains(item.Text, "背景") || strings.Contains(item.Text, "形状") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Japanese slide/shape text, got: %v", textsOf(items))
		}
	}

	// Ensure <a:tab> was not treated as text.
	for _, item := range items {
		if strings.Contains(item.Text, "tab pos") || strings.HasPrefix(item.Text, "<a:") {
			t.Errorf("extracted non-text content: %q", item.Text)
		}
	}
}

func TestExtract_DrawingMLWithAttributes(t *testing.T) {
	content := `<xdr:wsDr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<a:t xml:space="preserve">带属性的形状文字</a:t>
<a:t>普通文字</a:t>
<a:tab pos="0" algn="l"/>
<a:tailEnd type="triangle"/>
</xdr:wsDr>`
	e := NewExtractor(ExtractorConfig{})
	_, items, err := e.Extract(content, "xl/drawings/drawing1.xml")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d: %v", len(items), textsOf(items))
	}
	if items[0].Text != "带属性的形状文字" || items[1].Text != "普通文字" {
		t.Errorf("unexpected texts: %v", textsOf(items))
	}
}

func textsOf(items []ExtractionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}
