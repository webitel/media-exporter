package maroto

import (
	"fmt"
	"time"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/webitel/media-exporter/api/storage"
)

func GeneratePDF(files map[string]string, fileInfos map[string]*storage.File) ([]byte, error) {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetBorder(false)

	imageHeight := 230.0
	textHeight := 20.0

	type tmpFile struct {
		Path      string
		Name      string
		Timestamp string
	}

	tmpFiles := make([]tmpFile, 0, len(files))
	for id, path := range files {
		file := fileInfos[id]
		timestamp := "unknown"
		if file != nil && file.UploadedAt > 0 {
			t := time.Unix(file.UploadedAt/1000, (file.UploadedAt%1000)*1e6)
			timestamp = t.Format("15:04 02.01.2006")
		}
		tmpFiles = append(tmpFiles, tmpFile{
			Path:      path,
			Name:      file.Name,
			Timestamp: timestamp,
		})
	}

	for i, f := range tmpFiles {
		if f.Path == "" {
			continue
		}

		// --- Screenshot ---
		m.Row(imageHeight, func() {
			m.Col(12, func() {
				tryAddImage(m, f.Path)
			})
		})

		// --- Metadata ---
		m.Row(textHeight, func() {
			m.Col(12, func() {
				m.Text(
					fmt.Sprintf("%s\n%s", f.Name, f.Timestamp),
					props.Text{
						Size:  10,
						Align: consts.Center,
					},
				)
			})
		})

		// --- New page except last ---
		if i < len(tmpFiles)-1 {
			m.AddPage()
		}
	}

	buf, err := m.Output()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func tryAddImage(m pdf.Maroto, path string) {
	if path == "" {
		fmt.Println("Skipped empty path")
		return
	}
	if err := m.FileImage(path); err != nil {
		fmt.Println("Error adding image:", err, "path:", path)
	}
}
