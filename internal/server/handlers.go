package server

import (
	authHttp "github.com/amankumarsingh77/cloud-video-encoder/internal/auth/delivery/http"
	authRepository "github.com/amankumarsingh77/cloud-video-encoder/internal/auth/repository"
	authUsecase "github.com/amankumarsingh77/cloud-video-encoder/internal/auth/usecase"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/middleware"
	videoHttp "github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles/delivery/http"
	videoRepository "github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles/repository"
	videoUsecase "github.com/amankumarsingh77/cloud-video-encoder/internal/videofiles/usecase"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
	"github.com/labstack/echo/v4"
	"net/http"
)

func (s *Server) MapHandlers(e *echo.Echo) error {
	aRepo := authRepository.NewAuthRepo(s.db)
	nRepo := videoRepository.NewVideoRepo(s.db)
	vAWSRepo := videoRepository.NewAwsRepository(s.s3Client)
	vRedisRepo := videoRepository.NewVideoRedisRepo(s.redisClient)

	authUC := authUsecase.NewAuthUseCase(s.cfg, aRepo, s.logger)
	videoUC := videoUsecase.NewVideoUseCase(s.cfg, nRepo, vRedisRepo, vAWSRepo, s.logger)

	authHandlers := authHttp.NewAuthHandler(s.cfg, authUC, s.logger)
	videoHandlers := videoHttp.NewVideoHandler(videoUC)

	mw := middleware.NewMiddlewareManager(authUC, s.cfg, []string{"*"}, s.logger)

	v1 := e.Group("/api/v1")
	health := v1.Group("/health")
	authGroup := v1.Group("/auth")
	videoGroup := v1.Group("/video")

	authHttp.MapAuthRoutes(authGroup, authHandlers, mw, authUC, s.cfg)
	videoHttp.MapVideoRoutes(videoGroup, videoHandlers, mw)
	health.GET("", func(c echo.Context) error {
		s.logger.Infof("Health check RequestID: %s", utils.GetRequestID(c))
		return c.JSON(http.StatusOK, map[string]string{"status": "OK"})
	})
	return nil
}
