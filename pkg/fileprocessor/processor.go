package fileprocessor

import (
	"archive/zip"
	"exceltranslator/pkg/logger" // Import the logger package
	"exceltranslator/pkg/textextractor"
	"exceltranslator/pkg/translator"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileProcessor struct {
	extractor *textextractor.Extractor
	logger    *logger.Logger // Add logger instance
}

func NewFileProcessor() *FileProcessor {
	// Default logger if not explicitly provided
	return NewFileProcessorWithLogger(logger.NewLogger(100))
}

// NewFileProcessorWithLogger creates a new FileProcessor instance with a given logger.
func NewFileProcessorWithLogger(log *logger.Logger) *FileProcessor {
	return &FileProcessor{
		extractor: textextractor.NewExtractor(textextractor.ExtractorConfig{}), // Default empty config
		logger:    log,
	}
}

// SetExtractorConfig updates the configuration for the text extractor.
func (fp *FileProcessor) SetExtractorConfig(config textextractor.ExtractorConfig) {
	fp.extractor = textextractor.NewExtractor(config)
}

// ProcessFile processes the input docx/xlsx file and saves the translated version to outputPath.
// The translator performs translation operations and progress reporting.
func (fp *FileProcessor) ProcessFile(inputPath string, outputPath string, trans translator.Translator) error {
	fp.logger.Infof("Processing file: %s", inputPath)

	// Open the zip file
	r, err := zip.OpenReader(inputPath)
	if err != nil {
		fp.logger.Errorf("Failed to open source file %s: %v", inputPath, err)
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer r.Close()

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fp.logger.Errorf("Failed to create output directory %s: %v", filepath.Dir(outputPath), err)
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create the output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		fp.logger.Errorf("Failed to create output file %s: %v", outputPath, err)
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create a zip writer
	w := zip.NewWriter(outFile)
	defer w.Close()

	// Iterate through the files in the archive
	for _, f := range r.File {
		fp.logger.Tracef("Processing internal file: %s", f.Name)
		err := fp.processZipFile(f, w, trans)
		if err != nil {
			fp.logger.Errorf("Failed to process internal file %s: %v", f.Name, err)
			return fmt.Errorf("failed to process file %s: %w", f.Name, err)
		}
	}
	fp.logger.Tracef("Finished processing file: %s", inputPath)
	return nil
}

// processZipFile handles individual files within the zip archive.
// It applies translation if the file is an XML document requiring text extraction.
func (fp *FileProcessor) processZipFile(f *zip.File, w *zip.Writer, trans translator.Translator) error {
	// Open the file inside the zip
	rc, err := f.Open()
	if err != nil {
		fp.logger.Errorf("Failed to open file %s in zip: %v", f.Name, err)
		return fmt.Errorf("failed to open file in zip %s: %w", f.Name, err)
	}
	defer rc.Close()

	// Read content
	contentBytes, err := io.ReadAll(rc)
	if err != nil {
		fp.logger.Errorf("Failed to read content of %s: %v", f.Name, err)
		return fmt.Errorf("failed to read content of %s: %w", f.Name, err)
	}
	content := string(contentBytes)

	// Determine if this file needs processing
	isXmlFile := strings.HasSuffix(f.Name, ".xml")
	needsTranslation := false

	if isXmlFile {
		// Common for DOCX and XLSX
		if strings.Contains(f.Name, "word/document.xml") ||
			strings.Contains(f.Name, "word/header") ||
			strings.Contains(f.Name, "word/footer") ||
			strings.Contains(f.Name, "xl/sharedStrings.xml") ||
			strings.Contains(f.Name, "xl/drawings/drawing") ||
			strings.Contains(f.Name, "xl/comments") ||
			strings.Contains(f.Name, "xl/workbook.xml") {
			needsTranslation = true
		}
	}

	var newContent string
	if needsTranslation {
		fp.logger.Tracef("Extracting and translating text from %s", f.Name)

		// 1. Extract text
		cleanedContent, items, err := fp.extractor.Extract(content, f.Name)
		if err != nil {
			fp.logger.Errorf("Extraction failed for %s: %v", f.Name, err)
			return fmt.Errorf("extraction failed for %s: %w", f.Name, err)
		}

		// 2. Translate text batch
		texts := make([]string, len(items))
		for i, item := range items {
			texts[i] = item.Text
		}
		translations, err := trans.TranslateFileTexts(f.Name, texts)
		if err != nil {
			fp.logger.Errorf("Translation failed for %s: %v", f.Name, err)
			return fmt.Errorf("translation failed for %s: %w", f.Name, err)
		}

		// 3. Apply replacements
		newContent, err = fp.extractor.Apply(cleanedContent, f.Name, items, translations)
		if err != nil {
			fp.logger.Errorf("Replacement failed for %s: %v", f.Name, err)
			return fmt.Errorf("replacement failed for %s: %w", f.Name, err)
		}
		fp.logger.Tracef("Finished translating text from %s", f.Name)
	} else {
		newContent = content // No translation needed, use original content
		fp.logger.Tracef("No translation needed for %s, copying directly.", f.Name)
	}

	// Create a header for the new file in the zip writer, preserving original metadata
	header := &zip.FileHeader{
		Name:     f.Name,
		Method:   f.Method,
		Modified: f.Modified,
	}

	wWrapper, err := w.CreateHeader(header)
	if err != nil {
		fp.logger.Errorf("Failed to create zip entry for %s: %v", f.Name, err)
		return fmt.Errorf("failed to create zip entry for %s: %w", f.Name, err)
	}
	_, err = wWrapper.Write([]byte(newContent))
	if err != nil {
		fp.logger.Errorf("Failed to write content for %s to zip: %v", f.Name, err)
		return fmt.Errorf("failed to write content for %s to zip: %w", f.Name, err)
	}

	return nil
}
