package utils

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/labstack/echo/v4"
)

type UserCtxKey struct{}

func GetUserFromCtx(ctx context.Context) (*models.User, error) {
	user, ok := ctx.Value(UserCtxKey{}).(*models.User)
	if !ok {
		return nil, fmt.Errorf("user not found in context")
	}
	return user, nil
}

func GetRequestID(c echo.Context) string {
	return c.Response().Header().Get(echo.HeaderXRequestID)
}
