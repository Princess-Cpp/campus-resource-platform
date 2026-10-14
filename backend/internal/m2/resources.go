package m2

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
)

// HTTPResources consumes the existing M1 v1 contract, never M1 tables or service methods.
type HTTPResources struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (r *HTTPResources) call(ctx context.Context, method, path, token, key, trace string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		raw, e := json.Marshal(payload)
		if e != nil {
			return 0, nil, e
		}
		body = bytes.NewReader(raw)
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.BaseURL, "/")+"/api/v1"+path, body)
	if e != nil {
		return 0, nil, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if trace != "" {
		req.Header.Set("X-Correlation-Id", trace)
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, e := client.Do(req)
	if e != nil {
		return 0, nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return res.StatusCode, raw, e
}
func (r *HTTPResources) Available(ctx context.Context, token string, start, end time.Time) (map[string]bool, error) {
	q := url.Values{"startAt": {start.Format(time.RFC3339)}, "endAt": {end.Format(time.RFC3339)}}
	status, raw, e := r.call(ctx, "GET", "/resources/availability?"+q.Encode(), token, "", "", nil)
	if e != nil {
		return nil, e
	}
	if status != 200 {
		return nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	var body struct {
		Data []struct {
			Room struct {
				ID    string `json:"id"`
				Seats []struct {
					ID     string `json:"id"`
					Active bool   `json:"active"`
				} `json:"seats"`
			} `json:"room"`
			Seat struct {
				ID string `json:"id"`
			} `json:"seat"`
		} `json:"data"`
	}
	if e = json.Unmarshal(raw, &body); e != nil {
		return nil, e
	}
	counts := map[string]map[string]bool{}
	totals := map[string]int{}
	for _, v := range body.Data {
		if counts[v.Room.ID] == nil {
			counts[v.Room.ID] = map[string]bool{}
			for _, seat := range v.Room.Seats {
				if seat.Active {
					totals[v.Room.ID]++
				}
			}
		}
		counts[v.Room.ID][v.Seat.ID] = true
	}
	result := map[string]bool{}
	for room, seats := range counts {
		result[room] = totals[room] > 0 && len(seats) == totals[room]
	}
	return result, nil
}
func decodeReceipt(raw []byte) (Receipt, error) {
	var body struct {
		Data Receipt `json:"data"`
	}
	e := json.Unmarshal(raw, &body)
	if e != nil || body.Data.OperationID == "" {
		return Receipt{}, fmt.Errorf("invalid resource receipt")
	}
	return body.Data, nil
}
func (r *HTTPResources) Lookup(ctx context.Context, operation string) (Receipt, bool, error) {
	status, raw, e := r.call(ctx, "GET", "/internal/resources/operations/"+url.PathEscape(operation), r.Token, "", "", nil)
	if e != nil {
		return Receipt{}, false, e
	}
	if status == 404 {
		return Receipt{}, false, nil
	}
	if status != 200 {
		return Receipt{}, false, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	v, e := decodeReceipt(raw)
	return v, e == nil, e
}
func (r *HTTPResources) Execute(ctx context.Context, q Operation, trace string) (Receipt, error) {
	status, raw, e := r.call(ctx, "POST", "/internal/resources/operations", r.Token, q.OperationID, trace, q)
	if e != nil {
		return Receipt{}, e
	}
	if status != 200 && status != 409 {
		return Receipt{}, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	return decodeReceipt(raw)
}
