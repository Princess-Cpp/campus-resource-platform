package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

// ResourceRequest is M3's caller-side representation of the existing v1 wire
// contract. M3 never imports M1's persistence or writes its source tables.
type ResourceRequest struct {
	OperationID      string `json:"operationId"`
	Action           string `json:"action"`
	SourceModule     string `json:"sourceModule"`
	SourceBusinessID string `json:"sourceBusinessId"`
	SourceVersion    int64  `json:"sourceVersion"`
	RoomID           string `json:"roomId"`
	Reason           string `json:"reason"`
}
type ResourceReceipt struct {
	OperationID      string    `json:"operationId"`
	Status           string    `json:"operationStatus"`
	SourceModule     string    `json:"sourceModule"`
	SourceBusinessID string    `json:"sourceBusinessId"`
	SourceVersion    int64     `json:"sourceVersion"`
	EffectiveAt      time.Time `json:"effectiveAt"`
	ErrorCode        string    `json:"errorCode,omitempty"`
}

func (r ResourceReceipt) domainReceipt() domain.Receipt {
	return domain.Receipt{OperationID: r.OperationID, Status: r.Status, SourceModule: r.SourceModule, SourceBusinessID: r.SourceBusinessID, SourceVersion: r.SourceVersion, EffectiveAt: r.EffectiveAt, ErrorCode: r.ErrorCode}
}

type Resources interface {
	Lookup(context.Context, string) (ResourceReceipt, bool, error)
	Execute(context.Context, ResourceRequest, string) (ResourceReceipt, error)
}
type HTTPResources struct {
	base, token string
	client      *http.Client
}

func NewHTTPResources(base, token string) (*HTTPResources, error) {
	u, e := url.Parse(base)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("invalid M3 resource endpoint or credential configuration")
	}
	return &HTTPResources{base: strings.TrimRight(base, "/"), token: token, client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (h *HTTPResources) call(ctx context.Context, method, path string, body any, trace string) (ResourceReceipt, bool, error) {
	var payload []byte
	var e error
	if body != nil {
		payload, e = json.Marshal(body)
		if e != nil {
			return ResourceReceipt{}, false, e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, h.base+"/api/v1/internal/resources/operations"+path, bytes.NewReader(payload))
	if e != nil {
		return ResourceReceipt{}, false, e
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", body.(ResourceRequest).OperationID)
	}
	if keyPattern.MatchString(trace) {
		req.Header.Set("X-Correlation-Id", trace)
	}
	res, e := h.client.Do(req)
	if e != nil {
		return ResourceReceipt{}, false, fmt.Errorf("resource transport unavailable")
	}
	defer res.Body.Close()
	if method == http.MethodGet && res.StatusCode == 404 {
		return ResourceReceipt{}, false, nil
	}
	if res.StatusCode != 200 && !(method == http.MethodPost && res.StatusCode == 409) {
		return ResourceReceipt{}, false, fmt.Errorf("resource HTTP status %d", res.StatusCode)
	}
	var envelope struct {
		Data *ResourceReceipt `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 64<<10))
	if e = decoder.Decode(&envelope); e != nil || envelope.Data == nil {
		return ResourceReceipt{}, false, fmt.Errorf("invalid resource response")
	}
	if e = decoder.Decode(new(any)); e != io.EOF {
		return ResourceReceipt{}, false, fmt.Errorf("invalid resource response tail")
	}
	return *envelope.Data, true, nil
}
func (h *HTTPResources) Lookup(ctx context.Context, id string) (ResourceReceipt, bool, error) {
	return h.call(ctx, http.MethodGet, "/"+url.PathEscape(id), nil, "")
}
func (h *HTTPResources) Execute(ctx context.Context, q ResourceRequest, trace string) (ResourceReceipt, error) {
	r, _, e := h.call(ctx, http.MethodPost, "", q, trace)
	return r, e
}

// SourceReader uses the existing v1 sources endpoint; no provider table access.
type ResourceSource struct {
	SourceModule     string `json:"sourceModule"`
	SourceBusinessID string `json:"sourceBusinessId"`
	RoomID           string `json:"roomId"`
	Version          int64  `json:"sourceVersion"`
	Active           bool   `json:"active"`
	Kind             string `json:"kind"`
}
type SourceReader interface {
	Source(context.Context, string) (ResourceSource, error)
}

func (h *HTTPResources) Source(ctx context.Context, businessID string) (ResourceSource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+"/api/v1/internal/resources/sources", nil)
	if err != nil {
		return ResourceSource{}, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	res, err := h.client.Do(req)
	if err != nil {
		return ResourceSource{}, fmt.Errorf("resource source lookup unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ResourceSource{}, fmt.Errorf("resource source lookup status %d", res.StatusCode)
	}
	var envelope struct {
		Data []ResourceSource `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 4<<20))
	if err = decoder.Decode(&envelope); err != nil {
		return ResourceSource{}, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return ResourceSource{}, fmt.Errorf("invalid source response tail")
	}
	var found *ResourceSource
	for _, source := range envelope.Data {
		if source.SourceModule == "M3" && source.SourceBusinessID == businessID {
			if found != nil {
				return ResourceSource{}, fmt.Errorf("duplicate source")
			}
			value := source
			found = &value
		}
	}
	if found == nil || found.Version < 1 {
		return ResourceSource{}, fmt.Errorf("source missing or invalid")
	}
	return *found, nil
}
