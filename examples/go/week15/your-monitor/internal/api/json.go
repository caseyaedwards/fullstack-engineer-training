package api

// region: write

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
)

// writeJSON sends status with v encoded as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status) // after the headers: WriteHeader sends them
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write response", "err", err) // the client is gone
	}
}

// errorBody is the body of every error response: {"error": "..."}.
type errorBody struct {
	Error string `json:"error"`
}

// writeError sends status with {"error": msg}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// endregion: write

// region: decode

// maxBodyBytes caps a request body at 1 MiB. A monitor needs under 3 KB.
const maxBodyBytes = 1 << 20

// isJSON reports whether the request says its body is JSON. It accepts
// parameters, as in application/json; charset=utf-8.
func isJSON(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

// decodeJSON reads one JSON object from the body into dst. On failure it
// returns the status code to answer with and an error written for the
// client.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) (int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // a typo such as "interval" is an error, not ignored

	err := dec.Decode(dst)
	if err == nil {
		// One object only: after it, the body must end. Decode into an any,
		// which takes every JSON value, to see what follows.
		if err = dec.Decode(new(any)); err == io.EOF {
			return 0, nil
		}
		if err == nil {
			return http.StatusBadRequest, errors.New("body must hold a single JSON object")
		}
	}

	var tooBig *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooBig):
		return http.StatusRequestEntityTooLarge, fmt.Errorf("body is larger than %d bytes", tooBig.Limit)
	case errors.Is(err, io.EOF):
		return http.StatusBadRequest, errors.New("body is empty")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return http.StatusBadRequest, errors.New("body is not valid JSON")
	case errors.As(err, &typeErr) && typeErr.Field == "":
		return http.StatusBadRequest, fmt.Errorf("body must be a JSON object, got a JSON %s", typeErr.Value)
	case errors.As(err, &typeErr):
		return http.StatusBadRequest, fmt.Errorf("%s has the wrong type: got a JSON %s", typeErr.Field, typeErr.Value)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return http.StatusBadRequest, errors.New(strings.TrimPrefix(err.Error(), "json: "))
	default:
		return http.StatusBadRequest, err
	}
}

// endregion: decode
