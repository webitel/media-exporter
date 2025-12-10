package grpc

import (
	"context"
	"fmt"

	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/service"

	"github.com/webitel/media-exporter/internal/errors"
)

type PdfHandler struct {
	service       service.PdfService
	storageClient storage.FileServiceClient
	pdfapi.UnimplementedPdfServiceServer
}

func NewPdfHandler(service service.PdfService, storageClient storage.FileServiceClient) (*PdfHandler, error) {
	if service == nil || storageClient == nil {
		return nil, errors.Internal("PdfService or storageClient is nil")
	}
	return &PdfHandler{
		service:       service,
		storageClient: storageClient,
	}, nil
}

func mapDomainStatusToProto(status string) pdfapi.PdfExportStatus {
	switch status {
	case "done":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_DONE
	case "failed":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_FAILED
	case "pending":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_PENDING
	case "processing":
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_PROCESSING
	default:
		return pdfapi.PdfExportStatus_PDF_EXPORT_STATUS_UNSPECIFIED
	}
}

func ConvertToProtoHistoryResponse(internal *domain.HistoryResponse, limit, offset int64) *pdfapi.PdfHistoryResponse {
	if internal == nil {
		return &pdfapi.PdfHistoryResponse{}
	}

	protoRecords := make([]*pdfapi.PdfHistoryRecord, len(internal.Data))

	for i, rec := range internal.Data {
		protoRecords[i] = &pdfapi.PdfHistoryRecord{
			Id:        rec.ID,
			Name:      rec.Name,
			FileId:    rec.FileID,
			MimeType:  rec.MimeType,
			CreatedAt: rec.CreatedAt,
			UpdatedAt: rec.UpdatedAt,
			CreatedBy: rec.CreatedBy,
			UpdatedBy: rec.UpdatedBy,
			Status:    mapDomainStatusToProto(rec.Status),
		}
	}

	next := false
	if limit > 0 && (offset+int64(len(internal.Data)) < internal.Total) {
		next = true
	}

	return &pdfapi.PdfHistoryResponse{
		Data: protoRecords,
		Next: next,
	}
}

func (h *PdfHandler) GetPdfExportHistory(ctx context.Context, req *pdfapi.PdfHistoryRequest) (*pdfapi.PdfHistoryResponse, error) {
	opts, err := options.NewSearchOptions(ctx)
	if err != nil {
		return nil, err
	}

	reqOpts := &domain.PdfHistoryRequestOptions{
		AgentID: req.AgentId,
	}

	internalResponse, err := h.service.GetHistory(ctx, opts, reqOpts)
	if err != nil {
		return nil, err
	}

	return ConvertToProtoHistoryResponse(internalResponse, int64(req.Size), int64(req.Page)), nil
}

func (h *PdfHandler) GeneratePdfExport(ctx context.Context, req *pdfapi.PdfGenerateRequest) (*pdfapi.PdfExportMetadata, error) {
	opts, err := options.NewCreateOptions(ctx)
	if err != nil {
		return nil, err
	}

	metadata, err := h.service.GenerateExport(
		ctx,
		opts,
		req.AgentId,
		req.FileIds,
		int32(req.Channel),
		req.From,
		req.To,
	)
	if err != nil {
		return nil, err
	}

	return &pdfapi.PdfExportMetadata{
		TaskId:   metadata.TaskID,
		FileName: metadata.FileName,
		MimeType: metadata.MimeType,
		Status:   metadata.Status,
	}, nil
}

func (h *PdfHandler) DownloadPdfExport(req *pdfapi.PdfDownloadRequest, stream pdfapi.PdfService_DownloadPdfExportServer) error {
	return fmt.Errorf("DownloadPdfExport not implemented")
}

func (h *PdfHandler) DeletePdfExportRecord(ctx context.Context, req *pdfapi.DeletePdfExportRecordRequest) (*pdfapi.DeletePdfExportRecordResponse, error) {
	opts, err := options.NewDeleteOptions(ctx, []int64{req.Id})
	if err != nil {
		return nil, err
	}

	err = h.service.DeleteRecord(ctx, opts, req.Id)
	if err != nil {
		return nil, err
	}
	return &pdfapi.DeletePdfExportRecordResponse{Id: req.Id}, nil
}
