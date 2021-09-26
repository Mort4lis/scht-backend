package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Mort4lis/scht-backend/internal/config"

	"github.com/Mort4lis/scht-backend/internal/services"
	"github.com/Mort4lis/scht-backend/internal/utils"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/go-playground/validator/v10"
	"github.com/julienschmidt/httprouter"
)

type Handler struct {
	logger   logging.Logger
	validate *validator.Validate
}

func (h *Handler) DecodeJSONFromBody(body io.ReadCloser, decoder utils.JSONDecoder) error {
	if err := decoder.DecodeFrom(body); err != nil {
		h.logger.WithError(err).Debug("Invalid json body")
		return ErrInvalidJSON
	}

	defer func() {
		if err := body.Close(); err != nil {
			h.logger.WithError(err).Error("Error occurred while closing body")
		}
	}()

	return nil
}

func (h *Handler) Validate(s interface{}) error {
	if err := h.validate.Struct(s); err != nil {
		fields := ErrorFields{}
		for _, err := range err.(validator.ValidationErrors) {
			fields[err.Field()] = fmt.Sprintf(
				"field validation for '%s' failed on the '%s' tag",
				err.Field(), err.Tag(),
			)
		}

		return ResponseError{
			StatusCode: http.StatusBadRequest,
			Message:    "validation error",
			Fields:     fields,
		}
	}

	return nil
}

func ExtractTokenFromHeader(header string) (string, error) {
	logger := logging.GetLogger()

	if header == "" {
		logger.Debug("authorization header is empty")
		return "", ErrInvalidAuthorizationToken
	}

	headerParts := strings.Split(header, " ")
	if len(headerParts) != 2 {
		logger.Debug("authorization header must contains with two parts")
		return "", ErrInvalidAuthorizationToken
	}

	if headerParts[0] != "Bearer" {
		logger.Debug("authorization header doesn't begin with Bearer")
		return "", ErrInvalidAuthorizationToken
	}

	return headerParts[1], nil
}

func RespondSuccess(statusCode int, w http.ResponseWriter, encoder utils.JSONEncoder) {
	if encoder == nil {
		w.WriteHeader(statusCode)
		return
	}

	logger := logging.GetLogger()

	respBody, err := encoder.Encode()
	if err != nil {
		logger.WithError(err).Error("Error occurred while encoding response structure")
		RespondError(w, err)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if _, err = w.Write(respBody); err != nil {
		logger.WithError(err).Error("Error occurred while writing response body")
		return
	}
}

func RespondError(w http.ResponseWriter, err error) {
	appErr, ok := err.(ResponseError)
	if !ok {
		RespondError(w, ErrInternalServer)
		return
	}

	logger := logging.GetLogger()

	respBody, err := json.Marshal(appErr)
	if err != nil {
		logger.WithError(err).Error("Error occurred while marshalling application error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)

	if _, err = w.Write(respBody); err != nil {
		logger.WithError(err).Error("Error occurred while writing response body")
	}
}

func Init(container services.ServiceContainer, cfg *config.Config, validate *validator.Validate) http.Handler {
	router := httprouter.New()

	NewUserHandler(container.User, container.Auth, validate).Register(router)
	NewAuthHandler(container.Auth, validate, cfg.Domain, cfg.Auth.RefreshTokenTTL).Register(router)

	return router
}
