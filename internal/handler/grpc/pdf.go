package grpc

import (
	"context"

	pdfapi "github.com/webitel/media-exporter/api/pdf"
	"github.com/webitel/media-exporter/api/storage"
	"github.com/webitel/media-exporter/internal/domain/model/options"
	domain "github.com/webitel/media-exporter/internal/domain/model/pdf"
	"github.com/webitel/media-exporter/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// --- Screenrecording Exports ---

func (h *PdfHandler) CreateScreenrecordingExport(ctx context.Context, req *pdfapi.CreateScreenrecordingRequest) (*pdfapi.ExportTask, error) {
	if req.AgentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	opts, err := options.NewCreateOptions(ctx)
	if err != nil {
		return nil, err
	}

	metadata, err := h.service.GenerateExport(ctx, opts, &domain.GenerateExportRequest{
		AgentID: req.AgentId,
		FileIDs: req.FileIds,
		From:    req.From,
		To:      req.To,
	})
	if err != nil {
		return nil, err
	}

	return &pdfapi.ExportTask{
		TaskId:   metadata.TaskID,
		FileName: metadata.FileName,
		MimeType: metadata.MimeType,
		Status:   mapDomainStatusToProto(metadata.Status),
		Size:     metadata.Size,
	}, nil
}

func (h *PdfHandler) ListScreenrecordingExports(ctx context.Context, req *pdfapi.ListScreenrecordingHistoryRequest) (*pdfapi.ListExportsResponse, error) {
	if req.AgentId == 0 {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	internalResponse, err := h.service.GetHistory(ctx, &domain.PdfHistoryRequestOptions{
		AgentID: req.AgentId,
		Page:    req.Page,
		Size:    req.Size,
	})
	if err != nil {
		return nil, err
	}

	return convertToProtoHistoryResponse(internalResponse, int64(req.Size), int64(req.Page)), nil
}

// --- Call Exports ---

func (h *PdfHandler) CreateCallExport(ctx context.Context, req *pdfapi.CreateCallExportRequest) (*pdfapi.ExportTask, error) {
	if req.CallId == "" {
		return nil, status.Error(codes.InvalidArgument, "call_id is required")
	}

	opts, err := options.NewCreateOptions(ctx)
	if err != nil {
		return nil, err
	}

	metadata, err := h.service.GenerateCallExport(ctx, opts, &domain.GenerateCallExportRequest{
		CallID:  req.CallId,
		FileIDs: req.FileIds,
		From:    req.From,
		To:      req.To,
	})
	if err != nil {
		return nil, err
	}

	return &pdfapi.ExportTask{
		TaskId:   metadata.TaskID,
		FileName: metadata.FileName,
		MimeType: metadata.MimeType,
		Status:   mapDomainStatusToProto(metadata.Status),
		Size:     metadata.Size,
	}, nil
}

func (h *PdfHandler) ListCallExports(ctx context.Context, req *pdfapi.ListCallHistoryRequest) (*pdfapi.ListExportsResponse, error) {
	if req.CallId == "" {
		return nil, status.Error(codes.InvalidArgument, "call_id is required")
	}

	internalResponse, err := h.service.GetCallHistory(ctx, &domain.CallHistoryRequestOptions{
		CallID: req.CallId,
		Page:   req.Page,
		Size:   req.Size,
	})
	if err != nil {
		return nil, err
	}

	return convertToProtoHistoryResponse(internalResponse, int64(req.Size), int64(req.Page)), nil
}

// --- General Operations ---

func (h *PdfHandler) DeleteExport(ctx context.Context, req *pdfapi.DeleteExportRequest) (*pdfapi.DeleteExportResponse, error) {
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	opts, err := options.NewDeleteOptions(ctx, []int64{req.Id})
	if err != nil {
		return nil, err
	}

	err = h.service.DeleteRecord(ctx, opts, req.Id)
	if err != nil {
		return nil, err
	}
	return &pdfapi.DeleteExportResponse{Id: req.Id}, nil
}

// --- Mappers ---

func mapDomainStatusToProto(status string) pdfapi.ExportStatus {
	switch status {
	case "pending":
		return pdfapi.ExportStatus_PENDING
	case "processing":
		return pdfapi.ExportStatus_PROCESSING
	case "done":
		return pdfapi.ExportStatus_DONE
	case "failed":
		return pdfapi.ExportStatus_FAILED
	default:
		return pdfapi.ExportStatus_EXPORT_STATUS_UNSPECIFIED
	}
}

func convertToProtoHistoryResponse(internal *domain.HistoryResponse, limit, page int64) *pdfapi.ListExportsResponse {
	if internal == nil {
		return &pdfapi.ListExportsResponse{}
	}

	protoRecords := make([]*pdfapi.ExportRecord, len(internal.Data))
	for i, rec := range internal.Data {
		protoRecords[i] = &pdfapi.ExportRecord{
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
	hasNext := internal.Next

	return &pdfapi.ListExportsResponse{
		Page:  int32(page),
		Next:  hasNext,
		Items: protoRecords,
	}
}
