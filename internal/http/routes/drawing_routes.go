package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"botDashboard/internal/drawing"
	"botDashboard/internal/model"

	"github.com/go-www/silverlining"
)

func currentDrawingUser(ctx *silverlining.Context) (model.UserData, bool) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return model.UserData{}, false
	}
	return user, true
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
	items, err := client.ListImages(context.Background(), user)
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
	item, err := client.GetImage(context.Background(), user, id)
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
	body, contentType, err := client.DownloadImage(context.Background(), user, id)
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
			data, _ := io.ReadAll(part)
			if err := json.Unmarshal(data, &meta); err != nil {
				return drawingMetadata{}, nil, "", &Error{Message: err.Error(), Status: http.StatusBadRequest}
			}
		case "file":
			filename = part.FileName()
			data, _ := io.ReadAll(part)
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
	item, err := client.CreateImage(context.Background(), user, drawing.CreatePayload{
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
	item, err := client.UpdateImage(context.Background(), user, id, drawing.CreatePayload{
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
	if err := client.DeleteImage(context.Background(), user, id); err != nil {
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
func GetDrawingImages(ctx *silverlining.Context)         { getDrawingImages(ctx) }
func GetDrawingImage(ctx *silverlining.Context, id string) { getDrawingImage(ctx, id) }
func GetDrawingImageContent(ctx *silverlining.Context, id string) { getDrawingImageContent(ctx, id) }
func PostDrawingImage(ctx *silverlining.Context)          { postDrawingImage(ctx) }
func PutDrawingImage(ctx *silverlining.Context, id string) { putDrawingImage(ctx, id) }
func DeleteDrawingImage(ctx *silverlining.Context, id string) { deleteDrawingImage(ctx, id) }
