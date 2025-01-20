package http

import (
	"github.com/amankumarsingh77/cloud-video-encoder/internal/auth"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/config"
	"github.com/amankumarsingh77/cloud-video-encoder/internal/models"
	"github.com/amankumarsingh77/cloud-video-encoder/pkg/logger"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"net/http"
)

type authHandler struct {
	cfg    *config.Config
	authUc auth.UseCase
	logger logger.Logger
}

func NewAuthHandler(cfg *config.Config, authUc auth.UseCase, logger logger.Logger) auth.Handler {
	return &authHandler{
		cfg:    cfg,
		authUc: authUc,
		logger: logger,
	}
}

func (h *authHandler) Register() echo.HandlerFunc {
	return func(c echo.Context) error {
		user := &models.User{}
		if err := c.Bind(user); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		}

		createUser, err := h.authUc.Register(c.Request().Context(), user)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, createUser)
	}
}

func (h *authHandler) Login() echo.HandlerFunc {
	return func(c echo.Context) error {
		user := &models.User{}
		if err := c.Bind(user); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		}

		loginUser, err := h.authUc.Login(c.Request().Context(), user)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, loginUser)
	}
}

func (h *authHandler) GetMe() echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := c.Get("user").(*models.User)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		}
		return c.JSON(http.StatusOK, user)
	}
}

// TODO Implement reset password
func (h *authHandler) ResetPassword() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, "Reset Password")
	}
}

// TODO implement logout
func (h *authHandler) Logout() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, "Logout")
	}
}

func (h *authHandler) Update() echo.HandlerFunc {
	return func(c echo.Context) error {
		user := &models.User{}
		if err := c.Bind(user); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		}

		updateUser, err := h.authUc.Update(c.Request().Context(), user)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, updateUser)
	}
}

//func (h *authHandler) DeactivateUser() echo.HandlerFunc {
//	return func(c echo.Context) error {
//		userID, err := uuid.Parse(c.Param("id"))
//		if err != nil {
//			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user id"})
//		}
//		err = h.authUc.Delete(c.Request().Context(), userID)
//		if err != nil {
//			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
//		}
//		return c.JSON(http.StatusOK, map[string]string{"message": "User deleted successfully"})
//	}
//}

func (h *authHandler) GetUserByID() echo.HandlerFunc {
	return func(c echo.Context) error {

		uID, err := uuid.Parse(c.Param("user_id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user id"})
		}

		user, err := h.authUc.GetByID(c.Request().Context(), uID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, user)
	}
}

func (h *authHandler) GetUserStorageStats() echo.HandlerFunc {
	return func(c echo.Context) error {
		stats, err := h.authUc.GetUserStorageStats(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, stats)
	}
}

func (h *authHandler) GenerateApiKey() echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := c.Get("user").(*models.User)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		}
		apikey, err := h.authUc.GenerateApiKey(c.Request().Context(), user.UserID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"apikey": apikey})
	}
}
