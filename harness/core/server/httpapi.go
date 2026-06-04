package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

// principalHeader carries the AUTHENTICATED edge identity. The server trusts THIS, never the request body
// (D7/S9). In production an auth layer (mTLS/OIDC) sets it; httptest sets it from the edge's bound credential.
const principalHeader = "X-Mnemon-Principal"

type ingestResponse struct {
	Seq int64 `json:"seq"`
	Dup bool  `json:"dup"`
}

// NewHTTPHandler exposes a ServerAPI over net/http (D5: production HTTP/gRPC+mTLS is a thin adapter; this is
// the thin adapter, gated by httptest). The principal comes from principalHeader; the body carries only the
// observation. This is what makes "multi-machine" multi-execution-surface over real loopback HTTP — never
// multi-writer (the one ControlServer behind it stays the sole serializer).
func NewHTTPHandler(api ServerAPI) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		principal := contract.ActorID(r.Header.Get(principalHeader))
		if principal == "" {
			http.Error(w, "missing authenticated principal", http.StatusUnauthorized)
			return
		}
		var env contract.ObservationEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seq, dup, err := api.Ingest(principal, env)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingestResponse{Seq: seq, Dup: dup})
	})
	mux.HandleFunc("/projection", func(w http.ResponseWriter, r *http.Request) {
		principal := contract.ActorID(r.Header.Get(principalHeader))
		if principal == "" {
			http.Error(w, "missing authenticated principal", http.StatusUnauthorized)
			return
		}
		var sub contract.Subscription
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proj, err := api.PullProjection(principal, sub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden) // identity/scope violation
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(proj)
	})
	return mux
}

// Client is a thin edge-side HTTP client bound to one authenticated principal (its credential). It satisfies
// ServerAPI so an edge can speak to a remote server exactly as to an in-process one.
type Client struct {
	baseURL   string
	principal contract.ActorID
	http      *http.Client
}

func NewClient(baseURL string, principal contract.ActorID) *Client {
	return &Client{baseURL: baseURL, principal: principal, http: http.DefaultClient}
}

var _ ServerAPI = (*Client)(nil)

// Ingest POSTs the observation to the server. The principal argument is ignored: the client's identity is its
// bound credential (sent as the trusted header), never a per-call claim — an edge cannot forge another's id.
func (c *Client) Ingest(_ contract.ActorID, env contract.ObservationEnvelope) (int64, bool, error) {
	body, err := json.Marshal(env)
	if err != nil {
		return 0, false, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/ingest", bytes.NewReader(body))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set(principalHeader, string(c.principal))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, false, fmt.Errorf("ingest failed: %s: %s", resp.Status, string(b))
	}
	var out ingestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, false, err
	}
	return out.Seq, out.Dup, nil
}

// PullProjection fetches the actor's scoped view from the server. The principal argument is ignored: the
// subscription's actor is sent in the body and the server cross-checks it against the bound credential header,
// so an edge cannot pull another actor's scope (D7/S9).
func (c *Client) PullProjection(_ contract.ActorID, sub contract.Subscription) (projection.Projection, error) {
	body, err := json.Marshal(sub)
	if err != nil {
		return projection.Projection{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/projection", bytes.NewReader(body))
	if err != nil {
		return projection.Projection{}, err
	}
	req.Header.Set(principalHeader, string(c.principal))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return projection.Projection{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return projection.Projection{}, fmt.Errorf("pull failed: %s: %s", resp.Status, string(b))
	}
	var proj projection.Projection
	if err := json.NewDecoder(resp.Body).Decode(&proj); err != nil {
		return projection.Projection{}, err
	}
	return proj, nil
}
