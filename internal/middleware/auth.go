package middleware

import (
	"context"
	"errors"
	"fmt"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/httpErrors"
	"log"
	"net/http"
	"strings"

	"github.com/amankumarsingh77/cloud-video-encoder/internal/auth"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/utils"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type UserCtxKey struct {
}

func (mw *MiddlewareManager) AuthSessionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(mw.cfg.Session.Name)
		if err != nil {
			if errors.Is(err, http.ErrNoCookie) {
				return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(err))
			}
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}

		if cookie.Value == "" {
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}
		sess, err := mw.sessUC.GetSessionByID(context.Background(), cookie.Value)
		if err != nil {
			log.Println("reached")
			mw.logger.Errorf("GetSessionByID RequestID: %s, CookieValue: %s, Error: %s",
				utils.GetRequestID(c),
				cookie.Value,
				err.Error(),
			)
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}

		if sess == nil {
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}

		user, err := mw.authUC.GetByID(context.Background(), sess.UserID)
		if err != nil {
			mw.logger.Errorf("GetByID RequestID: %s, Error: %s",
				utils.GetRequestID(c),
				err.Error(),
			)
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}

		if user == nil {
			return c.JSON(http.StatusUnauthorized, httpErrors.NewUnauthorizedError(httpErrors.Unauthorized))
		}

		c.Set("sid", cookie.Value)
		c.Set("uid", sess.SessionID)
		c.Set("user", user)

		ctx := context.WithValue(c.Request().Context(), utils.UserCtxKey{}, user)
		c.SetRequest(c.Request().WithContext(ctx))

		return next(c)
	}
}

func (mw *MiddlewareManager) AuthJWTMiddleware(authUC auth.UseCase, cfg *config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			log.Println("reached")
			bearerHeader := c.Request().Header.Get("Authorization")

			mw.logger.Infof("auth middleware bearerHeader %s", bearerHeader)

			if bearerHeader != "" {
				headerParts := strings.Split(bearerHeader, " ")
				if len(headerParts) != 2 {
					mw.logger.Error("auth middleware", zap.String("headerParts", "len(headerParts) != 2"))
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				}

				tokenString := headerParts[1]

				if err := mw.validateJWTToken(tokenString, authUC, c, cfg); err != nil {
					mw.logger.Error("middleware validateJWTToken", zap.String("headerJWT", err.Error()))
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
				}

				return next(c)
			}

			cookie, err := c.Cookie("jwt-token")
			if err != nil {
				mw.logger.Errorf("c.Cookie", err.Error())
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			}

			if err = mw.validateJWTToken(cookie.Value, authUC, c, cfg); err != nil {
				mw.logger.Errorf("validateJWTToken", err.Error())
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			}
			return next(c)
		}
	}
}

func (mw *MiddlewareManager) validateJWTToken(tokenString string, authUC auth.UseCase, c echo.Context, cfg *config.Config) error {
	if tokenString == "" {
		return fmt.Errorf("invalid token string")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signin method %v", token.Header["alg"])
		}
		secret := []byte(cfg.Server.JwtSecretKey)
		return secret, nil
	})
	if err != nil {
		return err
	}

	if !token.Valid {
		return fmt.Errorf("invalid token string")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["id"].(string)
		if !ok {
			return fmt.Errorf("invalid jwt claims")
		}

		userUUID, err := uuid.Parse(userID)
		if err != nil {
			return err
		}

		u, err := authUC.GetByID(c.Request().Context(), userUUID)
		if err != nil {
			return err
		}

		log.Println(u)

		c.Set("user", u)

		ctx := context.WithValue(c.Request().Context(), UserCtxKey{}, u)
		// req := c.Request().WithContext(ctx)
		c.SetRequest(c.Request().WithContext(ctx))
	}
	return nil
}

func (mw *MiddlewareManager) OwnerOrAdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, ok := c.Get("user").(*models.User)
			if !ok {
				mw.logger.Errorf("Error c.Get(user) RequestID: %s, ERROR: %s,", utils.GetRequestID(c), "invalid user ctx")
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			}

			if user.Role == models.AdminRole {
				return next(c)
			}

			if user.UserID.String() != c.Param("user_id") {
				mw.logger.Errorf("Error c.Get(user) RequestID: %s, UserID: %s, ERROR: %s,",
					utils.GetRequestID(c),
					user.UserID.String(),
					"invalid user ctx",
				)
				return c.JSON(http.StatusForbidden, map[string]string{"error": "Forbidden"})
			}

			return next(c)
		}
	}
}

func (mw *MiddlewareManager) RoleBasedAuthMiddleware(roles []models.Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, ok := c.Get("user").(*models.User)
			if !ok {
				mw.logger.Errorf("Error c.Get(user) RequestID: %s, UserID: %s, ERROR: %s,",
					utils.GetRequestID(c),
					user.UserID.String(),
					"invalid user ctx",
				)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			}

			for _, role := range roles {
				if role == user.Role {
					return next(c)
				}
			}

			mw.logger.Errorf("Error c.Get(user) RequestID: %s, UserID: %s, ERROR: %s,",
				utils.GetRequestID(c),
				user.UserID.String(),
				"invalid user ctx",
			)

			return c.JSON(http.StatusForbidden, map[string]string{"error": "Forbidden"})
		}
	}
}
