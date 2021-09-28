package http

import (
	"net/http"
	"time"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/service"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/go-playground/validator/v10"
	"github.com/julienschmidt/httprouter"
)

const (
	signInURI  = "/api/auth/sign-in"
	refreshURI = "/api/auth/refresh"
)

const refreshCookieName = "refresh_token"

type authHandler struct {
	*baseHandler
	service service.AuthService
	logger  logging.Logger

	domain          string
	refreshTokenTTL time.Duration
}

func newAuthHandler(as service.AuthService, validate *validator.Validate, domain string, refreshTokenTTL time.Duration) *authHandler {
	logger := logging.GetLogger()

	return &authHandler{
		baseHandler: &baseHandler{
			logger:   logger,
			validate: validate,
		},
		service:         as,
		logger:          logger,
		domain:          domain,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (h *authHandler) register(router *httprouter.Router) {
	router.POST(signInURI, h.signIn)
	router.POST(refreshURI, h.refresh)
}

func (h *authHandler) signIn(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var dto domain.SignInDTO
	if err := h.decodeJSONFromBody(req.Body, &dto); err != nil {
		respondError(w, err)
		return
	}

	if err := h.validateStruct(dto); err != nil {
		respondError(w, err)
		return
	}

	dto.Fingerprint = req.Header.Get("X-Fingerprint")
	if dto.Fingerprint == "" {
		h.logger.Debug("X-Fingerprint header is empty")
		respondError(w, errEmptyFingerprintHeader)

		return
	}

	pair, err := h.service.SignIn(req.Context(), dto)
	if err != nil {
		switch err {
		case domain.ErrWrongCredentials:
			respondError(w, ResponseError{StatusCode: http.StatusUnauthorized, Message: err.Error()})
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    pair.RefreshToken,
		Path:     refreshURI,
		Domain:   h.domain,
		Expires:  time.Now().Add(h.refreshTokenTTL),
		HttpOnly: true,
	})

	respondSuccess(http.StatusOK, w, pair)
}

func (h *authHandler) refresh(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var dto domain.RefreshSessionDTO
	if cookie, err := req.Cookie(refreshCookieName); err == nil {
		dto.RefreshToken = cookie.Value
	} else if err = h.decodeJSONFromBody(req.Body, &dto); err != nil {
		respondError(w, err)
		return
	}

	if err := h.validateStruct(dto); err != nil {
		respondError(w, err)
		return
	}

	dto.Fingerprint = req.Header.Get("X-Fingerprint")
	if dto.Fingerprint == "" {
		h.logger.Debug("X-Fingerprint header is empty")
		respondError(w, errEmptyFingerprintHeader)

		return
	}

	pair, err := h.service.Refresh(req.Context(), dto)
	if err != nil {
		switch err {
		case domain.ErrSessionNotFound, domain.ErrUserNotFound, domain.ErrInvalidRefreshToken:
			respondError(w, errInvalidRefreshToken)
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	respondSuccess(http.StatusOK, w, pair)
}