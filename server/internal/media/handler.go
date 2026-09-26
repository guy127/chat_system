package media

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"smalltalk/internal/apperr"
	"smalltalk/internal/platform/httpx"
	"smalltalk/internal/room"
)

type Handler struct {
	svc   *Service
	rooms *room.Handler
}

func NewHandler(svc *Service, rooms *room.Handler) *Handler {
	return &Handler{svc: svc, rooms: rooms}
}

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/rooms/:id/images", h.rooms.RequireMember())
	g.POST("", h.upload)
	g.GET("/:imageId", h.get)
}

type imageView struct {
	ID          uuid.UUID `json:"id"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	SizeBytes   int       `json:"size_bytes"`
}

// upload takes multipart/form-data with the file in the "image" field.
func (h *Handler) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBytes+64<<10)
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.Error(c, apperr.ImageTooLarge)
			return
		}
		httpx.Error(c, apperr.Invalid(`send the image as multipart field "image"`))
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		httpx.Error(c, apperr.Invalid("could not read image"))
		return
	}
	if len(data) > MaxBytes {
		httpx.Error(c, apperr.ImageTooLarge)
		return
	}
	m := room.MemberFrom(c)
	img, err := h.svc.Upload(c.Request.Context(), m.RoomID, m.ID, data)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, imageView{
		ID: img.ID, ContentType: img.ContentType, Width: img.Width, Height: img.Height, SizeBytes: img.SizeBytes,
	})
}

func (h *Handler) get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("imageId"))
	if err != nil {
		httpx.Error(c, apperr.NotFound)
		return
	}
	img, rc, err := h.svc.Open(c.Request.Context(), room.MemberFrom(c).RoomID, id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	defer func() { _ = rc.Close() }()

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", img.ContentType) // sniffed at upload; only jpeg/png/gif
	hdr.Set("Content-Length", strconv.Itoa(img.SizeBytes))
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	hdr.Set("Cache-Control", "private, max-age=86400, immutable") // image bytes never change
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}
