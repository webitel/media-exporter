package maroto

import (
	"fmt"
	"sort"
	"time"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/webitel/media-exporter/api/storage"
)

// pdfItem represents a single PDF page item
type pdfItem struct {
	Path string
	Time time.Time
}

// GeneratePDF creates a PDF document containing only images from the provided file paths.
// Screenshots are sorted by UploadedAt (int64 Unix timestamp).
func GeneratePDF(
	files map[string]string,
	fileInfos map[string]*storage.File,
) ([]byte, error) {

	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetBorder(false)

	// A4 portrait height ~297mm, leave margins
	imageHeight := 250.0

	// --- Collect & normalize data ---
	items := make([]pdfItem, 0, len(files))

	for id, path := range files {
		if path == "" {
			continue
		}

		info, ok := fileInfos[id]
		if !ok || info == nil || info.UploadedAt == 0 {
			// Fallback: include but push to the end
			items = append(items, pdfItem{
				Path: path,
				Time: time.Time{},
			})
			continue
		}

		// Detect seconds vs milliseconds
		var t time.Time
		if info.UploadedAt > 1e12 {
			// milliseconds
			t = time.UnixMilli(info.UploadedAt)
		} else {
			// seconds
			t = time.Unix(info.UploadedAt, 0)
		}

		items = append(items, pdfItem{
			Path: path,
			Time: t,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no valid images found for PDF")
	}

	// --- Sort by time (newest → oldest) ---
	sort.Slice(items, func(i, j int) bool {
		return items[i].Time.After(items[j].Time)
	})

	// --- Build PDF ---
	for i, item := range items {
		m.Row(imageHeight, func() {
			m.Col(12, func() {
				tryAddImage(m, item.Path)
			})
		})

		// Add new page except last
		if i < len(items)-1 {
			m.AddPage()
		}
	}

	buf, err := m.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to generate output: %w", err)
	}

	return buf.Bytes(), nil
}

// tryAddImage attempts to load and place an image into the PDF.
func tryAddImage(m pdf.Maroto, path string) {
	if path == "" {
		return
	}

	// FileImage fits the image into the column automatically
	if err := m.FileImage(path); err != nil {
		// Log and continue
		fmt.Printf("Error adding image: %v, path: %s\n", err, path)
	}
}
