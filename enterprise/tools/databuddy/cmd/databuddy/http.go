package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/buildbuddy-io/buildbuddy/server/util/db"
	"github.com/buildbuddy-io/buildbuddy/server/util/proto"
	"github.com/buildbuddy-io/buildbuddy/server/util/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type httpError struct {
	err  error
	code int
}

func (e *httpError) Error() string {
	return e.err.Error()
}

func (e *httpError) Code() int {
	return e.code
}

func HTTPError(err error, code int) error {
	return &httpError{err: err, code: code}
}

func BadRequest(err error) *httpError {
	return &httpError{err: err, code: http.StatusBadRequest}
}

func NotFound(err error) *httpError {
	return &httpError{err: err, code: http.StatusNotFound}
}

func InternalServerError(err error) *httpError {
	return &httpError{err: err, code: http.StatusInternalServerError}
}

func APIHandler[T proto.Message](handler func(r *http.Request) (T, error)) http.Handler {
	return WithAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := handler(r)
		if err != nil {
			if httpErr, ok := err.(*httpError); ok {
				writeJSONError(w, httpErr.Code(), httpErr)
			} else if IsNotFound(err) {
				writeJSONError(w, http.StatusNotFound, fmt.Errorf("not found"))
			} else {
				writeJSONError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeProtoJSON(w, result)
	}))
}

func IsNotFound(err error) bool {
	return os.IsNotExist(errors.Unwrap(err)) || db.IsRecordNotFound(errors.Unwrap(err)) || status.IsNotFoundError(errors.Unwrap(err))
}

func writeProtoJSON(w http.ResponseWriter, message proto.Message) {
	b, err := protojson.Marshal(message)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Errorf("marshal: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
	}
}

func writeJSONError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); err != nil {
		log.Printf("Failed to write JSON error response: %v", err)
	}
}

func computeSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func ServeContentWithETagCaching(rs io.ReadSeeker) http.Handler {
	var precomputedHash string
	if br, ok := rs.(*bytes.Reader); ok {
		br.Seek(0, io.SeekStart)
		precomputedHash, _ = computeSHA256(br)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hash := precomputedHash
		if hash == "" {
			h, err := computeSHA256(rs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			hash = h
		}
		etag := `"` + hash + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		io.Copy(w, rs)
	})
}

func SetContentType(h http.Handler, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		h.ServeHTTP(w, r)
	})
}

type logErrorsWriter struct {
	http.ResponseWriter
	status              int
	logNextWriteAsError bool
}

func (w *logErrorsWriter) WriteHeader(status int) {
	w.status = status
	if status >= 500 {
		w.logNextWriteAsError = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *logErrorsWriter) Write(b []byte) (int, error) {
	// To avoid buffering, assume only one Write will occur.
	if w.logNextWriteAsError {
		w.logNextWriteAsError = false
		log.Printf("Error (HTTP %d): %s", w.status, strings.TrimSpace(string(b)))
	}
	// if w.status >= 500 {
	// 	// Make internal server errors opaque.
	// 	b = []byte("internal server error")
	// }
	return w.ResponseWriter.Write(b)
}

func LogServerErrors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(&logErrorsWriter{ResponseWriter: w}, r)
	})
}
