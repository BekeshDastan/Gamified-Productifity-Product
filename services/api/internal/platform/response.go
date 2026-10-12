package platform

import (
	"encoding/json"
	"net/http"
	"time"
)

type APIError struct {
	Status         int               `json:"-"`
	Code           string            `json:"code"`
	Message        string            `json:"message"`
	Fields         map[string]string `json:"fields,omitempty"`
	RequestID      string            `json:"request_id"`
	CurrentVersion *int64            `json:"current_version,omitempty"`
}

func (e *APIError) Error() string { return e.Code }

type Meta struct {
	RequestID  string `json:"request_id"`
	ServerTime string `json:"server_time"`
}
type Response[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

func WriteData[T any](w http.ResponseWriter, r *http.Request, c Clock, status int, data T) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response[T]{Data: data, Meta: Meta{RequestID: RequestMeta(r.Context()).RequestID, ServerTime: c.Now().UTC().Format(time.RFC3339Nano)}})
}
func WriteError(w http.ResponseWriter, r *http.Request, err *APIError) {
	copy := *err
	copy.RequestID = RequestMeta(r.Context()).RequestID
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(copy.Status)
	_ = json.NewEncoder(w).Encode(struct {
		Error APIError `json:"error"`
	}{copy})
}
