// Package app wires every package into one HTTP handler.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"smalltalk/internal/auth"
	"smalltalk/internal/chat"
	"smalltalk/internal/invite"
	"smalltalk/internal/media"
	"smalltalk/internal/platform/config"
	"smalltalk/internal/platform/httpx"
	"smalltalk/internal/room"
	"smalltalk/internal/service"
)

// NewRouter builds the API. Background work (rate-limiter eviction, image
// cleanup) stops when ctx is done.
func NewRouter(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, broker chat.Broker) (*gin.Engine, error) {
	issuer := auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL)
	hub := chat.NewHub(broker)

	roomSvc := room.NewService(room.NewRepository(pool), issuer, hub, hub)
	roomHandler := room.NewHandler(roomSvc, issuer)

	joinLimiter := httpx.NewKeyedLimiter(ctx, rate.Every(time.Minute/10), 10) // 10 joins/min/IP
	inviteSvc := invite.NewService(invite.NewRepository(pool), roomSvc, issuer, cfg.AppBaseURL)

	store, err := media.NewFSStorage(cfg.MediaDir)
	if err != nil {
		return nil, err
	}
	uploadLimiter := httpx.NewKeyedLimiter(ctx, rate.Every(6*time.Second), 5) // 10 images/min/member, bursts of 5
	mediaSvc := media.NewService(media.NewRepository(pool), store, roomSvc, uploadLimiter)
	go mediaSvc.RunCleanup(ctx, 10*time.Minute)

	sendLimiter := httpx.NewKeyedLimiter(ctx, 5, 5) // 5 messages/s/member
	chatSvc := chat.NewService(chat.NewRepository(pool), roomSvc, mediaSvc, broker, sendLimiter)
	serviceSvc := service.NewService(roomSvc, issuer, cfg.ServiceJWTTTL)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	r.Use(gin.Recovery(), httpx.RequestLog(), httpx.CORS(cfg.CORSOrigins))
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	roomHandler.Register(r)
	invite.NewHandler(inviteSvc, roomHandler, joinLimiter).Register(r)
	media.NewHandler(mediaSvc, roomHandler).Register(r)
	chat.NewHandler(chatSvc, hub, roomSvc, roomHandler, issuer, cfg.AllowedOrigins).Register(r)
	service.NewHandler(serviceSvc, cfg.ServiceAPIKey).Register(r)
	return r, nil
}
