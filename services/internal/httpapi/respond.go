package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(true)
		_ = enc.Encode(v)
	}
}

func ok(w http.ResponseWriter, v any) error { writeJSON(w, http.StatusOK, v); return nil }
func created(w http.ResponseWriter, v any) error { writeJSON(w, http.StatusCreated, v); return nil }
func noContent(w http.ResponseWriter) error { w.WriteHeader(http.StatusNoContent); return nil }

// Page is the envelope for paginated lists.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

const maxJSONBody = 2 << 20

// decode reads a single JSON object, rejecting unknown fields, trailing data and non-JSON content.
func decode(r *http.Request, dst any) error {
	return decodeLimit(r, dst, maxJSONBody)
}

func decodeLimit(r *http.Request, dst any, limit int64) error {
	ct := r.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if mt != "application/json" {
		return &APIError{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Content-Type must be application/json."}
	}
	body := http.MaxBytesReader(nil, r.Body, limit)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		var se *json.SyntaxError
		var te *json.UnmarshalTypeError
		switch {
		case errors.As(err, &mbe):
			return &APIError{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "The request body is too large."}
		case errors.As(err, &se), errors.Is(err, io.ErrUnexpectedEOF):
			return errBadRequest("The request body is not valid JSON.")
		case errors.As(err, &te):
			if te.Field != "" {
				return errField(te.Field, "has the wrong type")
			}
			return errBadRequest("The request body has a value of the wrong type.")
		case errors.Is(err, io.EOF):
			return errBadRequest("The request body is empty.")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return errBadRequest("Unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ") + ".")
		default:
			return errBadRequest("The request body could not be read: " + err.Error())
		}
	}
	if dec.More() {
		return errBadRequest("The request body must contain a single JSON object.")
	}
	return nil
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, errNotFound("Record")
	}
	return id, nil
}

// pagination parses limit and offset with safe bounds.
func pagination(r *http.Request) (int32, int32) {
	q := r.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	if offset > 1_000_000 {
		offset = 1_000_000
	}
	return int32(limit), int32(offset)
}

// Query helpers produce nil for absent filters so SQL can ignore them.
func qText(r *http.Request, k string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(k))
	if v == "" {
		return nil
	}
	if len(v) > 200 {
		v = v[:200]
	}
	return &v
}

func qUUID(r *http.Request, k string) (*uuid.UUID, error) {
	v := strings.TrimSpace(r.URL.Query().Get(k))
	if v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return nil, errField(k, "must be a UUID")
	}
	return &id, nil
}

func qBool(r *http.Request, k string) *bool {
	switch strings.ToLower(r.URL.Query().Get(k)) {
	case "true", "1", "yes":
		b := true
		return &b
	case "false", "0", "no":
		b := false
		return &b
	}
	return nil
}

func qInt(r *http.Request, k string) *int32 {
	v, err := strconv.Atoi(r.URL.Query().Get(k))
	if err != nil {
		return nil
	}
	i := int32(v)
	return &i
}

func qTime(r *http.Request, k string) (*time.Time, error) {
	v := r.URL.Query().Get(k)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, errField(k, "must be an RFC 3339 timestamp")
	}
	return &t, nil
}

// qSort returns the sort key if it is in the allow-list, otherwise "".
func qSort(r *http.Request, allowed ...string) string {
	v := r.URL.Query().Get("sort")
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return ""
}

func strPtr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
