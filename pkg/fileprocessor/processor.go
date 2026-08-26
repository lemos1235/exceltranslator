package fileprocessor

import (
	"archive/zip"
	"exceltranslator/pkg/logger"
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

type ExtractedDoc struct {
	xmlPath string
	xmlType string
	items   []textextractor.ExtractionItem
	texts   []string
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

// ProcessFileNew implements the new translation workflow: Extract -> Translate -> Replace -> Pack.
func (fp *FileProcessor) ProcessFileNew(inputPath string, outputPath string, trans translator.Translator) error {
	fp.logger.Infof("Processing file (New Workflow): %s", inputPath)

	// 1. Create temporary directory
	tempDir, err := os.MkdirTemp("", "exceltranslator_")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // Clean up

	fp.logger.Tracef("Created temp directory: %s", tempDir)

	// 2. Unzip all files to tempDir
	if err := unzipToDir(inputPath, tempDir); err != nil {
		return fmt.Errorf("failed to unzip file: %w", err)
	}

	// 3. Step 1: Extract Text
	var targets []string
	// Walk through the temp directory once to collect targets
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(tempDir, path)
		// Fix for windows paths in zip if necessary, but relPath should be fine.
		// Standardize separators to forward slashes for matching
		slashPath := filepath.ToSlash(relPath)

		if textextractor.ShouldExtractXML(slashPath) {
			targets = append(targets, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error during extraction phase: %w", err)
	}

	var docs []ExtractedDoc
	for _, path := range targets {
		relPath, _ := filepath.Rel(tempDir, path)
		slashPath := filepath.ToSlash(relPath)
		fp.logger.Tracef("Extracting text from: %s", slashPath)

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read xml file %s: %w", path, err)
		}
		content := string(contentBytes)

		cleanedContent, items, err := fp.extractor.Extract(content, slashPath)
		if err != nil {
			return err
		}

		// Replace original file with cleanedContent
		if err := os.WriteFile(path, []byte(cleanedContent), 0644); err != nil {
			return err
		}

		if len(items) == 0 {
			continue
		}

		texts := make([]string, 0, len(items))
		for _, item := range items {
			texts = append(texts, item.Text)
		}

		docs = append(docs, ExtractedDoc{
			xmlPath: path,
			xmlType: slashPath,
			items:   items,
			texts:   texts,
		})
	}

	// 4. Step 2: Execute Translation
	// 先统计全部内部文件的待翻译条目总数，用于上报整个文件的总体进度
	if setter, ok := trans.(translator.TotalProgressSetter); ok {
		totalTexts := 0
		for i := range docs {
			totalTexts += len(docs[i].texts)
		}
		setter.SetTotalTexts(totalTexts)
	}

	for i := range docs {
		doc := &docs[i]
		fp.logger.Tracef("Translating data for: %s", doc.xmlType)

		translations, err := trans.TranslateFileTexts(doc.xmlType, doc.texts)
		if err != nil {
			return err
		}
		if len(translations) != len(doc.items) {
			return fmt.Errorf("translation count mismatch for %s: got %d, want %d", doc.xmlType, len(translations), len(doc.items))
		}
		doc.texts = translations
	}

	// 5. Step 3: Execute Replacement
	for _, doc := range docs {
		fp.logger.Tracef("Applying translations to: %s", doc.xmlPath)

		xmlBytes, err := os.ReadFile(doc.xmlPath)
		if err != nil {
			return err
		}
		xmlContent := string(xmlBytes)

		newContent, err := fp.extractor.Apply(xmlContent, doc.xmlType, doc.items, doc.texts)
		if err != nil {
			return err
		}

		if err := os.WriteFile(doc.xmlPath, []byte(newContent), 0644); err != nil {
			return err
		}
	}

	// 6. Step 4: Pack
	if err := zipDir(tempDir, outputPath); err != nil {
		return fmt.Errorf("failed to pack output file: %w", err)
	}

	fp.logger.Infof("Finished processing file (New Workflow): %s", outputPath)
	return nil
}

func unzipToDir(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check for Zip Slip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func zipDir(srcDir, destZip string) error {
	zipFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes for zip files
		relPath = filepath.ToSlash(relPath)

		wWrapper, err := w.CreateHeader(&zip.FileHeader{
			Name:     relPath,
			Method:   zip.Deflate, // Use Deflate for compression
			Modified: info.ModTime(),
		})
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(wWrapper, f)
		return err
	})
}
