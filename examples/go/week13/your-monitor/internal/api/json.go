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

// decodeJSON reads one JSON object from the body into a new T. On failure
// it returns nil, the status code to answer with, and an error written for
// the client. T is a type parameter: decodeJSON[createRequest] reads a
// createRequest.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (*T, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // a typo such as "interval" is an error, not ignored

	var v *T // a body of null leaves v nil
	err := dec.Decode(&v)
	if err == nil {
		// One object only: after it, the body must end. Decode into an any,
		// which takes every JSON value, to see what follows.
		err = dec.Decode(new(any))
		switch {
		case err == nil:
			return nil, http.StatusBadRequest, errors.New("body must hold a single JSON object")
		case err == io.EOF && v == nil:
			return nil, http.StatusBadRequest, errors.New("body must be a JSON object, got a JSON null")
		case err == io.EOF:
			return v, 0, nil
		}
	}
	status, msg := decodeError(err)
	return nil, status, msg
}

// decodeError turns an error from Decode into a status code and a message
// for the client.
func decodeError(err error) (int, error) {
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
