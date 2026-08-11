package model

import (
	"net/http"
)

// AppError is the API error response shape shared by every handler.
// Code/Message mirror the TrueMoney-style JSON error bodies consumers
// expect ("code" + "message"). Status is the real HTTP status code and
// is never serialized.
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *AppError) Error() string { return e.Message }

// Sentinel errors used across the API. Bad client input is answered with
// HTTP 200 + code/message in the body, matching the upstream convention
// the frontend was built against.
var (
	ErrBadRequest = &AppError{Code: http.StatusBadRequest, Message: "Bad Request", Status: http.StatusOK}
	ErrNotFound   = &AppError{Code: http.StatusNotFound, Message: "Not Found", Status: http.StatusNotFound}
	ErrInternal   = &AppError{Code: http.StatusInternalServerError, Message: "Internal Server Error", Status: http.StatusOK}
)