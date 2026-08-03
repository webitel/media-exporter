package maroto

import (
	"fmt"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/webitel/media-exporter/api/storage"
)

// GeneratePDF creates a PDF document containing only images from the provided file paths.
func GeneratePDF(
	files []*storage.File,
	paths map[string]string,
) ([]byte, error) {

	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetBorder(false)

	// A4 portrait height ~297mm, leave margins
	imageHeight := 250.0

	items := orderedPaths(files, paths)
	if len(items) == 0 {
		return nil, fmt.Errorf("no valid images found for PDF")
	}

	// --- Build PDF ---
	for i, path := range items {
		p := path
		m.Row(imageHeight, func() {
			m.Col(12, func() {
				tryAddImage(m, p)
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

// orderedPaths returns downloaded paths in the files order.
func orderedPaths(files []*storage.File, paths map[string]string) []string {
	res := make([]string, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		if p := paths[fmt.Sprint(f.Id)]; p != "" {
			res = append(res, p)
		}
	}
	return res
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
