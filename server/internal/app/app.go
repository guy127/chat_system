// Package app wires every package into one HTTP handler.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"qrchat/internal/auth"
	"qrchat/internal/chat"
	"qrchat/internal/invite"
	"qrchat/internal/platform/config"
	"qrchat/internal/platform/httpx"
	"qrchat/internal/room"
)

// NewRouter builds the API. Background work (rate-limiter eviction) stops when ctx is done.
func NewRouter(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, broker chat.Broker) (*gin.Engine, error) {
	issuer := auth.NewIssuer(cfg.JWTSecret, cfg.JWTTTL)
	hub := chat.NewHub(broker)

	roomSvc := room.NewService(room.NewRepository(pool), issuer, hub, hub)
	roomHandler := room.NewHandler(roomSvc, issuer)

	joinLimiter := httpx.NewKeyedLimiter(ctx, rate.Every(time.Minute/10), 10) // 10 joins/min/IP
	inviteSvc := invite.NewService(invite.NewRepository(pool), roomSvc, issuer, cfg.AppBaseURL)

	sendLimiter := httpx.NewKeyedLimiter(ctx, 5, 5) // 5 messages/s/member
	chatSvc := chat.NewService(chat.NewRepository(pool), roomSvc, broker, sendLimiter)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	r.Use(gin.Recovery(), httpx.RequestLog())
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	roomHandler.Register(r)
	invite.NewHandler(inviteSvc, roomHandler, joinLimiter).Register(r)
	chat.NewHandler(chatSvc, hub, roomSvc, roomHandler, issuer, cfg.AllowedOrigins).Register(r)
	return r, nil
}
