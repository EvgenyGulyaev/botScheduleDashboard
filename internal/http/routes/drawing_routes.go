package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"botDashboard/internal/drawing"
	"botDashboard/internal/model"

	"github.com/go-www/silverlining"
)

func currentDrawingUser(ctx *silverlining.Context) (model.UserData, bool) {
	return requireDrawingAccess(ctx)
}

func drawingClient(ctx *silverlining.Context) (*drawing.Client, bool) {
	client := drawing.GetClient()
	if client == nil {
		GetError(ctx, &Error{Message: "drawing service is not configured", Status: http.StatusServiceUnavailable})
		return nil, false
	}
	return client, true
}

func getDrawingImages(ctx *silverlining.Context) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := client.ListImages(callCtx, user)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, map[string]any{"items": items}); err != nil {
		log.Print(err)
	}
}

func getDrawingImage(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.GetImage(callCtx, user, id)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func getDrawingImageContent(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, contentType, err := client.DownloadImage(callCtx, user, id)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	defer body.Close()
	if contentType != "" {
		ctx.ResponseHeaders().Set("Content-Type", contentType)
	}
	if err := ctx.WriteStream(http.StatusOK, body); err != nil {
		log.Print(err)
	}
}

type drawingMetadata struct {
	Title  string `json:"title"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func readDrawingMetadata(ctx *silverlining.Context) (drawingMetadata, io.Reader, string, *Error) {
	reader, err := ctx.MultipartReader()
	if err != nil {
		return drawingMetadata{}, nil, "", &Error{Message: "expected multipart/form-data", Status: http.StatusBadRequest}
	}
	var meta drawingMetadata
	var file io.Reader
	var filename string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return drawingMetadata{}, nil, "", &Error{Message: err.Error(), Status: http.StatusBadRequest}
		}
		name := part.FormName()
		switch name {
		case "metadata":
			data, _ := io.ReadAll(io.LimitReader(part, maxMetadataBytes+1))
			if int64(len(data)) > maxMetadataBytes {
				return drawingMetadata{}, nil, "", &Error{Message: "metadata too large", Status: http.StatusBadRequest}
			}
			if err := json.Unmarshal(data, &meta); err != nil {
				return drawingMetadata{}, nil, "", &Error{Message: err.Error(), Status: http.StatusBadRequest}
			}
		case "file":
			filename = part.FileName()
			data, _ := io.ReadAll(io.LimitReader(part, maxGatewayFileBytes+1))
			if int64(len(data)) > maxGatewayFileBytes {
				return drawingMetadata{}, nil, "", &Error{Message: "file is too large", Status: http.StatusRequestEntityTooLarge}
			}
			file = bytes.NewReader(data)
		default:
			part.Close()
		}
	}
	if file == nil {
		return drawingMetadata{}, nil, "", &Error{Message: "file is required", Status: http.StatusBadRequest}
	}
	return meta, file, filename, nil
}

const (
	maxMetadataBytes    int64 = 8 * 1024
	maxGatewayFileBytes int64 = 10 * 1024 * 1024
)

func postDrawingImage(ctx *silverlining.Context) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	meta, body, _, drawErr := readDrawingMetadata(ctx)
	if drawErr != nil {
		GetError(ctx, drawErr)
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.CreateImage(callCtx, user, drawing.CreatePayload{
		Title:  meta.Title,
		Width:  meta.Width,
		Height: meta.Height,
		Body:   body,
	})
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func putDrawingImage(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	meta, body, _, drawErr := readDrawingMetadata(ctx)
	if drawErr != nil {
		GetError(ctx, drawErr)
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.UpdateImage(callCtx, user, id, drawing.CreatePayload{
		Title:  meta.Title,
		Width:  meta.Width,
		Height: meta.Height,
		Body:   body,
	})
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func deleteDrawingImage(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.DeleteImage(callCtx, user, id); err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	ctx.WriteHeader(http.StatusNoContent)
}

func getDrawingStamps(ctx *silverlining.Context) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := client.ListStamps(callCtx, user)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, map[string]any{"items": items}); err != nil {
		log.Print(err)
	}
}

func getDrawingStamp(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.GetStamp(callCtx, user, id)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func getDrawingStampContent(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, contentType, err := client.DownloadStampImage(callCtx, user, id)
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	defer body.Close()
	if contentType != "" {
		ctx.ResponseHeaders().Set("Content-Type", contentType)
	}
	if err := ctx.WriteStream(http.StatusOK, body); err != nil {
		log.Print(err)
	}
}

type drawingStampMetadata struct {
	Name        string `json:"name"`
	TextValue   string `json:"textValue"`
	Priority    string `json:"priority"`
	RemoveImage bool   `json:"removeImage"`
}

func readDrawingStampMetadata(ctx *silverlining.Context) (drawingStampMetadata, io.Reader, string, string, *Error) {
	reader, err := ctx.MultipartReader()
	if err != nil {
		return drawingStampMetadata{}, nil, "", "", &Error{Message: "expected multipart/form-data", Status: http.StatusBadRequest}
	}
	var meta drawingStampMetadata
	var file io.Reader
	var filename string
	var mimeType string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return drawingStampMetadata{}, nil, "", "", &Error{Message: err.Error(), Status: http.StatusBadRequest}
		}
		name := part.FormName()
		switch name {
		case "metadata":
			data, _ := io.ReadAll(io.LimitReader(part, maxMetadataBytes+1))
			if int64(len(data)) > maxMetadataBytes {
				return drawingStampMetadata{}, nil, "", "", &Error{Message: "metadata too large", Status: http.StatusBadRequest}
			}
			if err := json.Unmarshal(data, &meta); err != nil {
				return drawingStampMetadata{}, nil, "", "", &Error{Message: err.Error(), Status: http.StatusBadRequest}
			}
		case "file":
			filename = part.FileName()
			mimeType = part.Header.Get("Content-Type")
			data, _ := io.ReadAll(io.LimitReader(part, maxGatewayFileBytes+1))
			if int64(len(data)) > maxGatewayFileBytes {
				return drawingStampMetadata{}, nil, "", "", &Error{Message: "file is too large", Status: http.StatusRequestEntityTooLarge}
			}
			file = bytes.NewReader(data)
		default:
			part.Close()
		}
	}
	return meta, file, filename, mimeType, nil
}

func postDrawingStamp(ctx *silverlining.Context) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	meta, body, filename, mimeType, drawErr := readDrawingStampMetadata(ctx)
	if drawErr != nil {
		GetError(ctx, drawErr)
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.CreateStamp(callCtx, user, drawing.StampPayload{
		Name:      meta.Name,
		TextValue: meta.TextValue,
		Priority:  meta.Priority,
		Body:      body,
		Filename:  filename,
		MimeType:  mimeType,
	})
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func putDrawingStamp(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	meta, body, filename, mimeType, drawErr := readDrawingStampMetadata(ctx)
	if drawErr != nil {
		GetError(ctx, drawErr)
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	item, err := client.UpdateStamp(callCtx, user, id, drawing.StampPayload{
		Name:        meta.Name,
		TextValue:   meta.TextValue,
		Priority:    meta.Priority,
		RemoveImage: meta.RemoveImage,
		Body:        body,
		Filename:    filename,
		MimeType:    mimeType,
	})
	if err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, item); err != nil {
		log.Print(err)
	}
}

func deleteDrawingStamp(ctx *silverlining.Context, id string) {
	user, ok := currentDrawingUser(ctx)
	if !ok {
		return
	}
	client, ok := drawingClient(ctx)
	if !ok {
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.DeleteStamp(callCtx, user, id); err != nil {
		writeUpstreamError(ctx, err)
		return
	}
	ctx.WriteHeader(http.StatusNoContent)
}

func writeUpstreamError(ctx *silverlining.Context, err error) {
	if dErr, ok := err.(*drawing.Error); ok {
		status := dErr.Status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		GetError(ctx, &Error{Message: dErr.Message, Status: status})
		return
	}
	GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
}

// public aliases for the server dispatcher
func GetDrawingImages(ctx *silverlining.Context)                  { getDrawingImages(ctx) }
func GetDrawingImage(ctx *silverlining.Context, id string)        { getDrawingImage(ctx, id) }
func GetDrawingImageContent(ctx *silverlining.Context, id string) { getDrawingImageContent(ctx, id) }
func PostDrawingImage(ctx *silverlining.Context)                  { postDrawingImage(ctx) }
func PutDrawingImage(ctx *silverlining.Context, id string)        { putDrawingImage(ctx, id) }
func DeleteDrawingImage(ctx *silverlining.Context, id string)     { deleteDrawingImage(ctx, id) }
func GetDrawingStamps(ctx *silverlining.Context)                  { getDrawingStamps(ctx) }
func GetDrawingStamp(ctx *silverlining.Context, id string)        { getDrawingStamp(ctx, id) }
func GetDrawingStampContent(ctx *silverlining.Context, id string) { getDrawingStampContent(ctx, id) }
func PostDrawingStamp(ctx *silverlining.Context)                  { postDrawingStamp(ctx) }
func PutDrawingStamp(ctx *silverlining.Context, id string)        { putDrawingStamp(ctx, id) }
func DeleteDrawingStamp(ctx *silverlining.Context, id string)     { deleteDrawingStamp(ctx, id) }
