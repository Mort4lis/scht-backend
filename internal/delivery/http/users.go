package http

import (
	"encoding/json"
	"net/http"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/service"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/go-playground/validator/v10"
	"github.com/julienschmidt/httprouter"
)

const (
	currentUserURL  = "/api/user"
	userPasswordURI = "/api/user/password"
	listUserURL     = "/api/users"
	detailUserURI   = "/api/users/:id"
)

type UserListResponse struct {
	List []domain.User `json:"list"`
}

func (r UserListResponse) Encode() ([]byte, error) {
	return json.Marshal(r)
}

type userHandler struct {
	*baseHandler
	userService service.UserService
	authService service.AuthService
	logger      logging.Logger
}

func newUserHandler(us service.UserService, as service.AuthService, validate *validator.Validate) *userHandler {
	logger := logging.GetLogger()

	return &userHandler{
		baseHandler: &baseHandler{
			logger:   logger,
			validate: validate,
		},
		userService: us,
		authService: as,
		logger:      logger,
	}
}

func (h *userHandler) register(router *httprouter.Router) {
	router.GET(listUserURL, authorizationMiddleware(h.list, h.authService))
	router.POST(listUserURL, h.create)
	router.GET(detailUserURI, authorizationMiddleware(h.detail, h.authService))
	router.PUT(currentUserURL, authorizationMiddleware(h.update, h.authService))
	router.PUT(userPasswordURI, authorizationMiddleware(h.updatePassword, h.authService))
	router.DELETE(currentUserURL, authorizationMiddleware(h.delete, h.authService))
}

// @Summary Get list of users
// @Tags Users
// @Security JWTTokenAuth
// @Accept json
// @Produce json
// @Success 200 {object} UserListResponse
// @Failure 500 {object} ResponseError
// @Router /users [get]
func (h *userHandler) list(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	users, err := h.userService.List(req.Context())
	if err != nil {
		respondError(w, errInternalServer)
		return
	}

	respondSuccess(http.StatusOK, w, UserListResponse{List: users})
}

// @Summary Get user by id
// @Tags Users
// @Security JWTTokenAuth
// @Accept json
// @Produce json
// @Param id path string true "User id"
// @Success 200 {object} domain.User
// @Failure 404 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /users/{id} [get]
func (h *userHandler) detail(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	user, err := h.userService.GetByID(req.Context(), params.ByName("id"))
	if err != nil {
		switch err {
		case domain.ErrUserNotFound:
			respondError(w, ResponseError{StatusCode: http.StatusNotFound, Message: err.Error()})
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	respondSuccess(http.StatusOK, w, &user)
}

// @Summary Create user
// @Tags Users
// @Accept json
// @Produce json
// @Param input body domain.CreateUserDTO true "Create body"
// @Success 201 {object} domain.User
// @Failure 400 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /users [post]
func (h *userHandler) create(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	dto := domain.CreateUserDTO{}
	if err := h.decodeJSONFromBody(req.Body, &dto); err != nil {
		respondError(w, err)
		return
	}

	if err := h.validateStruct(dto); err != nil {
		respondError(w, err)
		return
	}

	user, err := h.userService.Create(req.Context(), dto)
	if err != nil {
		switch err {
		case domain.ErrUserUniqueViolation:
			respondError(w, ResponseError{StatusCode: http.StatusBadRequest, Message: err.Error()})
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	respondSuccess(http.StatusCreated, w, &user)
}

// @Summary Update current authenticated user
// @Security JWTTokenAuth
// @Tags Users
// @Accept json
// @Produce json
// @Param input body domain.UpdateUserDTO true "Update body"
// @Success 200 {object} domain.User
// @Failure 400,404 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /user [put]
func (h *userHandler) update(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	dto := domain.UpdateUserDTO{}
	if err := h.decodeJSONFromBody(req.Body, &dto); err != nil {
		respondError(w, err)
		return
	}

	dto.ID = domain.UserIDFromContext(req.Context())

	if err := h.validateStruct(dto); err != nil {
		respondError(w, err)
		return
	}

	user, err := h.userService.Update(req.Context(), dto)
	if err != nil {
		switch err {
		case domain.ErrUserUniqueViolation:
			respondError(w, ResponseError{StatusCode: http.StatusBadRequest, Message: err.Error()})
		case domain.ErrUserNotFound:
			respondError(w, ResponseError{StatusCode: http.StatusNotFound, Message: err.Error()})
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	respondSuccess(http.StatusOK, w, &user)
}

// @Summary Update current authenticated user's password
// @Security JWTTokenAuth
// @Tags Users
// @Accept json
// @Produce json
// @Param input body domain.UpdateUserPasswordDTO true "Update body"
// @Success 204 "No Content"
// @Failure 400,404 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /user/password [put]
func (h *userHandler) updatePassword(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	dto := domain.UpdateUserPasswordDTO{}
	if err := h.decodeJSONFromBody(req.Body, &dto); err != nil {
		respondError(w, err)
		return
	}

	dto.UserID = domain.UserIDFromContext(req.Context())

	if err := h.validateStruct(dto); err != nil {
		respondError(w, err)
		return
	}

	if err := h.userService.UpdatePassword(req.Context(), dto); err != nil {
		switch err {
		case domain.ErrWrongCurrentPassword:
			respondError(w, ResponseError{StatusCode: http.StatusBadRequest, Message: err.Error()})
		case domain.ErrUserNotFound:
			respondError(w, ResponseError{StatusCode: http.StatusNotFound, Message: err.Error()})
		}

		return
	}

	respondSuccess(http.StatusNoContent, w, nil)
}

// @Summary Delete current authenticated user
// @Security JWTTokenAuth
// @Tags Users
// @Accept json
// @Produce json
// @Success 204 "No Content"
// @Failure 404 {object} ResponseError
// @Failure 500 {object} ResponseError
// @Router /user [delete]
func (h *userHandler) delete(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	err := h.userService.Delete(req.Context(), domain.UserIDFromContext(req.Context()))
	if err != nil {
		switch err {
		case domain.ErrUserNotFound:
			respondError(w, ResponseError{StatusCode: http.StatusNotFound, Message: err.Error()})
		default:
			respondError(w, errInternalServer)
		}

		return
	}

	respondSuccess(http.StatusNoContent, w, nil)
}
