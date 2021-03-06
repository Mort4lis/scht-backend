package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/encoding"
	"github.com/Mort4lis/scht-backend/internal/service"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/Mort4lis/scht-backend/pkg/validator"
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

	domain          string
	refreshTokenTTL time.Duration
}

func newAuthHandler(as service.AuthService, domain string, refreshTokenTTL time.Duration) *authHandler {
	return &authHandler{
		baseHandler:     &baseHandler{logger: logging.GetLogger()},
		service:         as,
		domain:          domain,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (h *authHandler) register(router *httprouter.Router) {
	router.HandlerFunc(http.MethodPost, signInURI, h.signIn)
	router.HandlerFunc(http.MethodPost, refreshURI, h.refresh)
}

// @Summary user authentication
// @Tags Auth
// @Description Authentication user by username and password. Successful
// response includes http-only cookie with refresh token.
// @Accept json
// @Produce json
// @Param fingerprint header string true "Fingerprint header"
// @Param input body domain.SignInDTO true "Credentials body"
// @Success 200 {object} domain.JWTPair
// @Failure 400,401 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /auth/sign-in [post]
func (h *authHandler) signIn(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	dto := domain.SignInDTO{Fingerprint: req.Header.Get("X-Fingerprint")}

	if err := h.decodeBody(req.Body, encoding.NewJSONSignInDTOUnmarshaler(&dto)); err != nil {
		respondError(ctx, w, err)
		return
	}

	logFields := logging.Fields{
		"username":    dto.Username,
		"fingerprint": dto.Fingerprint,
	}
	ctx = logging.NewContextFromLogger(ctx, h.logger.WithFields(logFields))

	if dto.Fingerprint == "" {
		respondError(ctx, w, errEmptyFingerprintHeader)
		return
	}

	if err := h.validate(validator.StructValidator(dto)); err != nil {
		respondError(ctx, w, err)
		return
	}

	pair, err := h.service.SignIn(ctx, dto)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrWrongCredentials):
			respondError(ctx, w, errWrongCredentials.Wrap(err))
		default:
			respondError(ctx, w, err)
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

	respondSuccess(ctx, http.StatusOK, w, encoding.NewJSONTokenPairMarshaler(pair))
}

// @Summary refresh authorization token
// @Tags Auth
// @Description Successful response includes http-only cookie with refresh token.
// @Accept json
// @Produce json
// @Param fingerprint header string true "Fingerprint header"
// @Param input body domain.RefreshSessionDTO true "Refresh token body"
// @Success 200 {object} domain.JWTPair
// @Failure 400 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /auth/refresh [post]
func (h *authHandler) refresh(w http.ResponseWriter, req *http.Request) {
	dto := domain.RefreshSessionDTO{Fingerprint: req.Header.Get("X-Fingerprint")}
	if cookie, err := req.Cookie(refreshCookieName); err == nil {
		dto.RefreshToken = cookie.Value
	} else if err = h.decodeBody(req.Body, encoding.NewJSONRefreshSessionDTOUnmarshaler(&dto)); err != nil {
		respondError(req.Context(), w, err)
		return
	}

	logFields := logging.Fields{"fingerprint": dto.Fingerprint}
	ctx := logging.NewContextFromLogger(req.Context(), h.logger.WithFields(logFields))

	if dto.Fingerprint == "" {
		respondError(ctx, w, errEmptyFingerprintHeader)
		return
	}

	if err := h.validate(validator.StructValidator(dto)); err != nil {
		respondError(ctx, w, err)
		return
	}

	pair, err := h.service.Refresh(req.Context(), dto)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidRefreshToken):
			respondError(ctx, w, errInvalidRefreshToken.Wrap(err))
		default:
			respondError(ctx, w, err)
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

	respondSuccess(ctx, http.StatusOK, w, encoding.NewJSONTokenPairMarshaler(pair))
}
