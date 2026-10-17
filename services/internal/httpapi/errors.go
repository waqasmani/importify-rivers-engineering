package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// APIError is an error with an HTTP status and a stable machine-readable code.
type APIError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func errBadRequest(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}
func errNotFound(what string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: what + " not found"}
}
func errUnauthorized(msg string) *APIError {
	return &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: msg}
}
func errForbidden(msg string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}
func errConflict(code, msg string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: code, Message: msg}
}
func errValidation(fields valid.Errors) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Some fields are invalid.", Fields: fields}
}
func errField(field, msg string) *APIError {
	return errValidation(valid.Errors{field: msg})
}
func errUnavailable(code, msg string) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: code, Message: msg}
}

// checkValid converts a validator into an error.
func checkValid(v *valid.V) error {
	if v.OK() {
		return nil
	}
	return errValidation(v.Errs)
}

// constraintMessages turn database constraint names into user-facing messages.
var constraintMessages = map[string]string{
	"users_email_key":                              "A user with this e-mail address already exists.",
	"roles_key_key":                                "A role with this key already exists.",
	"workspaces_slug_key":                          "A workspace with this slug already exists.",
	"tags_name_key":                                "A tag with this name already exists.",
	"pipelines_entity_type_name_key":               "A pipeline with this name already exists.",
	"pipeline_stages_pipeline_id_name_key":         "A stage with this name already exists in the pipeline.",
	"custom_field_definitions_entity_type_key_key": "A custom field with this key already exists.",
	"source_adapters_key_key":                      "An adapter with this key already exists.",
	"ai_provider_configs_name_key":                 "A provider with this name already exists.",
	"product_images_product_id_url_key":            "This image is already attached to the product.",
	"users_single_owner":                           "There can only be one owner.",
}

// toAPIError maps arbitrary errors to API errors.
func toAPIError(err error) *APIError {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae
	}
	if store.IsNotFound(err) {
		return errNotFound("Record")
	}
	if errors.Is(err, context.Canceled) {
		return &APIError{Status: 499, Code: "client_closed_request", Message: "The request was cancelled."}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &APIError{Status: http.StatusGatewayTimeout, Code: "timeout", Message: "The operation took too long."}
	}
	code, constraint := store.PgErrorCode(err)
	switch code {
	case "23505":
		msg := constraintMessages[constraint]
		if msg == "" {
			msg = "A record with the same unique value already exists."
		}
		return errConflict("duplicate", msg)
	case "23503":
		if strings.Contains(err.Error(), "update or delete on table") {
			return errConflict("in_use", "This record is still referenced by other records.")
		}
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_reference", Message: "A referenced record does not exist."}
	case "23514":
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "constraint_violation", Message: fmt.Sprintf("A value is outside the allowed range (%s).", constraint)}
	case "22P02", "22007", "22008", "22003", "22001":
		return errBadRequest("A value has an invalid format or is too large.")
	}
	return nil
}

// h adapts error-returning handlers.
func (s *Server) h(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			s.writeError(w, r, err)
		}
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	ae := toAPIError(err)
	if ae == nil {
		s.log.ErrorContext(r.Context(), "request failed", slog.String("request_id", requestID(r)), slog.String("error", err.Error()))
		ae = &APIError{Status: http.StatusInternalServerError, Code: "internal_error",
			Message: "An unexpected error occurred. Reference: " + requestID(r)}
	} else if ae.Status >= 500 && ae.Status != 503 {
		s.log.ErrorContext(r.Context(), "request failed", slog.String("request_id", requestID(r)), slog.String("error", err.Error()))
	}
	writeJSON(w, ae.Status, map[string]any{"error": ae})
}
