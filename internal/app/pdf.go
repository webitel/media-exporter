package app

import (
	"fmt"
	"os"
	"time"

	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
	"github.com/webitel/media-exporter/api/storage"
)

func generatePDF(files map[string]string, fileInfos map[string]*storage.File) ([]byte, error) {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetBorder(false)

	imageHeight := 60.0
	imageSpacing := 10.0
	textHeight := 10.0

	type tmpFile struct {
		Path      string
		Name      string
		Timestamp string
	}

	tmpFiles := make([]tmpFile, 0, len(files))
	for id, path := range files {
		file := fileInfos[id]
		timestamp := "unknown"
		if file.UploadedAt > 0 {
			t := time.Unix(file.UploadedAt/1000, (file.UploadedAt%1000)*1e6)
			timestamp = t.Format("15:04 02.01.2006")
		}
		tmpFiles = append(tmpFiles, tmpFile{Path: path, Name: file.Name, Timestamp: timestamp})
	}

	for i := 0; i < len(tmpFiles); i += 2 {
		m.Row(imageHeight, func() {
			m.Col(6, func() { tryAddImage(m, tmpFiles[i].Path) })
			if i+1 < len(tmpFiles) {
				m.Col(6, func() { tryAddImage(m, tmpFiles[i+1].Path) })
			}
		})
		m.Row(textHeight, func() {
			m.Col(6, func() {
				m.Text(tmpFiles[i].Name+"\n"+tmpFiles[i].Timestamp, props.Text{Size: 10, Align: consts.Center})
			})
			if i+1 < len(tmpFiles) {
				m.Col(6, func() {
					m.Text(tmpFiles[i+1].Name+"\n"+tmpFiles[i+1].Timestamp, props.Text{Size: 10, Align: consts.Center})
				})
			}
		})
		m.Row(imageSpacing, func() {})
	}

	buf, err := m.Output()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func tryAddImage(m pdf.Maroto, path string) {
	if err := m.FileImage(path); err != nil {
		fmt.Println("Error adding image:", err)
	}
}

func savePDFToFile(pdfBytes []byte, filename string) error {
	return os.WriteFile(filename, pdfBytes, 0644)
}
