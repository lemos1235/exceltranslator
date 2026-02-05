package textextractor

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	phoneticRunRegex      = regexp.MustCompile(`(?s)<rPh\b[^>]*?>.*?</rPh>`)
	phoneticPropertyRegex = regexp.MustCompile(`(?s)<phoneticPr\b[^>]*?/?>`)
)

// FileType represents the type of file being processed
type FileType string

const (
	FileTypeDocx FileType = "docx"
	FileTypeXlsx FileType = "xlsx"
)

// ExtractorConfig holds configuration for the extraction process
type ExtractorConfig struct {
	CJKOnly bool // If true, only translate text containing CJK characters
}

// Extractor handles text extraction and replacement
type Extractor struct {
	config ExtractorConfig
}

// NewExtractor creates a new Extractor instance
func NewExtractor(config ExtractorConfig) *Extractor {
	return &Extractor{
		config: config,
	}
}

// ShouldExtractXML determines if an XML file should be processed by the extractor.
func ShouldExtractXML(fileName string) bool {
	if !strings.HasSuffix(fileName, ".xml") {
		return false
	}
	// Common for DOCX and XLSX
	if strings.Contains(fileName, "word/document.xml") ||
		strings.Contains(fileName, "word/header") ||
		strings.Contains(fileName, "word/footer") ||
		strings.Contains(fileName, "xl/sharedStrings.xml") ||
		strings.Contains(fileName, "xl/drawings/drawing") ||
		strings.Contains(fileName, "xl/comments") ||
		strings.Contains(fileName, "xl/workbook.xml") {
		return true
	}
	return false
}

// ContainsCJK checks if the string contains any CJK characters
func ContainsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || // Chinese
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul
			return true
		}
	}
	return false
}

// IsValidTextContent checks if the text is valid for translation.
// It returns false for empty strings, pure numbers, or text consisting only of symbols/punctuation.
func IsValidTextContent(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}

	// Check if it's just numbers and punctuation
	isMeaningful := false
	for _, r := range trimmed {
		if !unicode.IsNumber(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			isMeaningful = true
			break
		}
	}
	return isMeaningful
}

// ExtractionItem represents a text segment to be translated
type ExtractionItem struct {
	Text       string // The content to be translated
	MatchStart int    // Start index of the full XML match
	MatchEnd   int    // End index of the full XML match
	TextStart  int    // Start index of the text content within the match
	TextEnd    int    // End index of the text content within the match
}

// Extract finds text nodes in the content that need translation.
// It returns the (potentially modified) content and a list of ExtractionItems.
func (e *Extractor) Extract(content string, xmlType string) (string, []ExtractionItem, error) {
	var re *regexp.Regexp

	// DOCX - word/document.xml, word/header*.xml, word/footer*.xml
	if strings.Contains(xmlType, "word/document.xml") || strings.Contains(xmlType, "word/header") || strings.Contains(xmlType, "word/footer") {
		//<w:t xml:space="preserve">Hello there! My name is McKenzie, and I studied abroad at United International College in Zhuhai in the fall semester of 2023. I</w:t>
		re = regexp.MustCompile(`(?s)<w:t\b[^>]*?>(.*?)</w:t>`)
	} else if strings.Contains(xmlType, "xl/sharedStrings.xml") {
		// Clean up phonetic annotations (furigana/ruby) which should not be translated
		content = removePhoneticAnnotations(content)
		// XLSX Shared Strings
		re = regexp.MustCompile(`(?s)<t>(.*?)</t>`)
	} else if strings.Contains(xmlType, "xl/drawings/drawing") {
		// XLSX Drawings (Shapes)
		re = regexp.MustCompile(`(?s)<a:t>(.*?)</a:t>`)
	} else if strings.Contains(xmlType, "xl/comments") {
		re = regexp.MustCompile(`(?s)<t>(.*?)</t>`)
	} else if strings.Contains(xmlType, "xl/workbook.xml") {
		// XLSX Workbook - sheet names
		re = regexp.MustCompile(`<sheet name="([^"]+?)"[^>]*?>`)
	} else {
		return content, nil, nil // No translation needed
	}

	// Find all matches
	matches := re.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil, nil
	}

	var items []ExtractionItem

	// Extract
	for _, match := range matches {
		// match[0], match[1]: indices of the full match (e.g. <w:t>text</w:t>)
		// match[2], match[3]: indices of the capture group (e.g. text)

		if len(match) < 4 {
			continue
		}

		originalText := content[match[2]:match[3]]

		// Unescape XML entities before processing
		unescaped := html.UnescapeString(originalText)

		// 1. Filter: Check if text is meaningful (not just numbers/symbols)
		if !IsValidTextContent(unescaped) {
			continue
		}

		// 2. Filter: CJK Only check
		if e.config.CJKOnly && !ContainsCJK(unescaped) {
			continue
		}

		items = append(items, ExtractionItem{
			Text:       unescaped,
			MatchStart: match[0],
			MatchEnd:   match[1],
			TextStart:  match[2],
			TextEnd:    match[3],
		})
	}

	return content, items, nil
}

// Apply replaces the extracted items with their translations in the content.
func (e *Extractor) Apply(content string, xmlType string, items []ExtractionItem, translations []string) (string, error) {
	if len(items) != len(translations) {
		return "", fmt.Errorf("items count (%d) and translations count (%d) do not match", len(items), len(translations))
	}

	if len(items) == 0 {
		return content, nil
	}

	var sb strings.Builder
	sb.Grow(len(content))

	lastIndex := 0

	for i, item := range items {
		translated := translations[i]

		// For sheet names, Excel has a 31-character limit
		if strings.Contains(xmlType, "xl/workbook.xml") {
			translated = truncateSheetName(translated)
		}

		// Escape XML entities after translation
		escapedTranslated := html.EscapeString(translated)

		sb.WriteString(content[lastIndex:item.MatchStart])
		sb.WriteString(content[item.MatchStart:item.TextStart])
		sb.WriteString(escapedTranslated)
		sb.WriteString(content[item.TextEnd:item.MatchEnd])
		lastIndex = item.MatchEnd
	}

	// Append remaining content
	sb.WriteString(content[lastIndex:])

	return sb.String(), nil
}

// removePhoneticAnnotations strips Excel phonetic (ruby) markup that should not be preserved.
func removePhoneticAnnotations(content string) string {
	content = phoneticRunRegex.ReplaceAllString(content, "")
	content = phoneticPropertyRegex.ReplaceAllString(content, "")
	return content
}

// truncateSheetName enforces Excel's 31-character sheet name limit using rune count.
func truncateSheetName(name string) string {
	const maxRunes = 31
	runes := []rune(name)
	if len(runes) <= maxRunes {
		return name
	}
	return string(runes[:maxRunes])
}
