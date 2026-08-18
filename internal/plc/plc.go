package plc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lacsar712/spillway/internal/hashutil"
	"github.com/lacsar712/spillway/internal/idgen"
	"github.com/lacsar712/spillway/internal/sign"
)

type Result struct {
	Status int
	Error  error
	Body   string
}

type Client struct {
	http    *http.Client
	timeout time.Duration
}

func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}
	return &Client{
		timeout: timeout,
		http: &http.Client{
			Timeout:   timeout,
			Transport: t,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 1 {
					return http.ErrUseLastResponse
				}
				if len(via) == 0 {
					return nil
				}
				orig := via[0].URL
				if !sameHost(orig, req.URL) {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

func sameHost(a, b *url.URL) bool {
	return strings.EqualFold(a.Hostname(), b.Hostname()) && a.Port() == b.Port() && a.Scheme == b.Scheme
}

type Request struct {
	URL        string
	Secret     string
	CommandID  string
	DeliveryID string
	PLCID      string
	Attempt    int
	Body       []byte
	Now        time.Time
}

func (c *Client) Post(ctx context.Context, req Request) Result {
	if req.Attempt < 1 {
		req.Attempt = 1
	}
	nonce := idgen.New("n", req.Now)
	nonce = strings.TrimPrefix(nonce, "n_")
	if len(nonce) < 16 {
		nonce = nonce + strings.Repeat("0", 16-len(nonce))
	}
	if len(nonce) > 64 {
		nonce = nonce[:64]
	}
	unix := req.Now.Unix()
	sig, err := sign.Sign(req.Secret, unix, nonce, req.Body)
	if err != nil {
		return Result{Error: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return Result{Error: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Spill-Timestamp", fmt.Sprintf("%d", unix))
	httpReq.Header.Set("X-Spill-Nonce", nonce)
	httpReq.Header.Set("X-Spill-Signature", sig)
	httpReq.Header.Set("X-Spill-Command-Id", req.CommandID)
	httpReq.Header.Set("X-Spill-Delivery-Id", req.DeliveryID)
	httpReq.Header.Set("X-Spill-Attempt", fmt.Sprintf("%d", req.Attempt))
	httpReq.Header.Set("X-Spill-PLC", req.PLCID)
	httpReq.Header.Set("X-Spill-Body-Sha256", hashutil.SHA256Hex(req.Body))

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{Error: fmt.Errorf("plc write: %v", err)}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 2048)
	b, _ := io.ReadAll(limited)
	return Result{Status: resp.StatusCode, Body: string(b)}
}
