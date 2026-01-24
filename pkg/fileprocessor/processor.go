package fileprocessor

import (
	"archive/zip"
	"encoding/json"
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

// needsTranslation determines if a file needs to be processed.
func (fp *FileProcessor) needsTranslation(fileName string) bool {
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
	// Walk through the temp directory
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

		if fp.needsTranslation(slashPath) {
			fp.logger.Tracef("Extracting text from: %s", slashPath)

			contentBytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(contentBytes)

			cleanedContent, items, err := fp.extractor.Extract(content, slashPath)
			if err != nil {
				return err
			}

			// Replace original file with cleanedContent
			// (Note: Currently Extract returns original content as cleanedContent if no modification logic exists in Extract)
			if err := os.WriteFile(path, []byte(cleanedContent), info.Mode()); err != nil {
				return err
			}

			if len(items) > 0 {
				// Create xxx.xml.itemsidx
				idxPath := path + ".itemsidx"
				idxData, err := json.Marshal(items)
				if err != nil {
					return err
				}
				if err := os.WriteFile(idxPath, idxData, 0644); err != nil {
					return err
				}

				// Create xxx.xml.itemsdat
				datPath := path + ".itemsdat"
				var texts []string
				for _, item := range items {
					texts = append(texts, item.Text)
				}
				datData, err := json.Marshal(texts)
				if err != nil {
					return err
				}
				if err := os.WriteFile(datPath, datData, 0644); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error during extraction phase: %w", err)
	}

	// 4. Step 2: Execute Translation
	// Iterate all *.itemsdat files
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".itemsdat") {
			fp.logger.Tracef("Translating data file: %s", path)
			relPath, _ := filepath.Rel(tempDir, path)
			slashPath := filepath.ToSlash(relPath)
			fileName := strings.TrimSuffix(slashPath, ".itemsdat")
			if err := translateTextFile(fileName, path, trans); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error during translation phase: %w", err)
	}

	// 5. Step 3: Execute Replacement
	// Iterate all *.itemsidx files
	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".itemsidx") {
			idxPath := path
			xmlPath := strings.TrimSuffix(idxPath, ".itemsidx")
			datPath := xmlPath + ".itemsdat"

			fp.logger.Tracef("Applying translations to: %s", xmlPath)

			// Read items
			idxBytes, err := os.ReadFile(idxPath)
			if err != nil {
				return err
			}
			var items []textextractor.ExtractionItem
			if err := json.Unmarshal(idxBytes, &items); err != nil {
				return err
			}

			// Read translations
			datBytes, err := os.ReadFile(datPath)
			if err != nil {
				return err
			}
			var translations []string
			if err := json.Unmarshal(datBytes, &translations); err != nil {
				return err
			}

			// Read XML content
			xmlBytes, err := os.ReadFile(xmlPath)
			if err != nil {
				return err
			}
			xmlContent := string(xmlBytes)

			// Get relative path for xmlType
			relPath, _ := filepath.Rel(tempDir, xmlPath)
			xmlType := filepath.ToSlash(relPath)

			// Apply
			newContent, err := fp.extractor.Apply(xmlContent, xmlType, items, translations)
			if err != nil {
				return err
			}

			// Write back
			if err := os.WriteFile(xmlPath, []byte(newContent), 0644); err != nil {
				return err
			}

			// Delete temp files
			os.Remove(idxPath)
			os.Remove(datPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error during replacement phase: %w", err)
	}

	// 6. Step 4: Pack
	if err := zipDir(tempDir, outputPath); err != nil {
		return fmt.Errorf("failed to pack output file: %w", err)
	}

	fp.logger.Infof("Finished processing file (New Workflow): %s", outputPath)
	return nil
}

func translateTextFile(fileName, filePath string, trans translator.Translator) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	var texts []string
	if err := json.Unmarshal(content, &texts); err != nil {
		return fmt.Errorf("failed to parse JSON from %s: %w", filePath, err)
	}

	translatedTexts, err := trans.TranslateFileTexts(fileName, texts)
	if err != nil {
		return err
	}

	output, err := json.Marshal(translatedTexts)
	if err != nil {
		return fmt.Errorf("failed to marshal translated texts: %w", err)
	}

	if err := os.WriteFile(filePath, output, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

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
