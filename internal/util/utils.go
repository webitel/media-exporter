package util

import (
	"context"
	"os"
	"strings"

	"github.com/disintegration/imaging"
	"google.golang.org/grpc/metadata"
)

func IsValidImageMime(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/bmp":
		return true
	default:
		return false
	}
}

func CleanupFiles(files map[string]string) {
	for _, path := range files {
		_ = os.Remove(path)
	}
}

func ResizeImage(path string, width int) error {
	img, err := imaging.Open(path)
	if err != nil {
		return err
	}
	resized := imaging.Resize(img, width, 0, imaging.Lanczos)
	return imaging.Save(resized, path)
}

func SavePDFToTemp(path string, pdfBytes []byte) error {
	err := os.WriteFile(path, pdfBytes, 0644)
	if err != nil {
		return err
	}

	return nil
}

// build a new context.Background() with outgoing metadata created from headers map
func ContextWithHeaders(headers map[string]string) context.Context {
	ctx := context.Background()
	if len(headers) == 0 {
		return ctx
	}
	pairs := make([]string, 0, len(headers)*2)
	for k, v := range headers {
		pairs = append(pairs, k, v)
	}
	md := metadata.Pairs(pairs...)
	return metadata.NewOutgoingContext(ctx, md)
}

func GetFileExt(mime string) string {
	switch mime {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "application/pdf":
		return ".pdf"
	default:
		if i := strings.LastIndex(mime, "/"); i != -1 && i < len(mime)-1 {
			return "." + mime[i+1:]
		}
		return ""
	}
}
