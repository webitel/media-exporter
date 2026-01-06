package maroto

import (
	"fmt"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/webitel/media-exporter/api/storage"
)

// GeneratePDF creates a PDF document containing only images from the provided file paths.
func GeneratePDF(files map[string]string, fileInfos map[string]*storage.File) ([]byte, error) {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetBorder(false)

	// Define the height for the image row (A4 portrait height is ~297mm)
	imageHeight := 250.0

	// Collect non-empty file paths into a slice for indexed iteration
	paths := make([]string, 0, len(files))
	for _, path := range files {
		if path != "" {
			paths = append(paths, path)
		}
	}

	for i, path := range paths {
		// --- Add Image Row ---
		m.Row(imageHeight, func() {
			m.Col(12, func() {
				tryAddImage(m, path)
			})
		})

		// --- Add new page except for the last element ---
		if i < len(paths)-1 {
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
		// Log the error to stdout but continue processing other images
		fmt.Printf("Error adding image: %v, path: %s\n", err, path)
	}
}
