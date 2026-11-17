package mas

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/odango-chan/ca-client-tools/internal/sbi"
)

const (
	// smfContextServiceKey is the trusted service key used by the shared SBI
	// transport. It is private adapter configuration and is never exposed to a
	// caller.
	smfContextServiceKey = "openamsm-smf-context"
	// smfContextTokenProfile is the trusted header-token profile name.
	smfContextTokenProfile = "openamsm-query-token"
)

// sbiTransport migrates the typed SMF MAS client onto the shared internal/sbi
// HTTP/2 transport. The typed client keeps its query validation, response
// identity binding, status handling and ProblemDetails semantics; only the
// h2c connection management, timeout, cancellation and I/O bounds move to the
// shared transport.
type sbiTransport struct {
	client *sbi.Client
}

func newSBITransport(origin *url.URL, queryToken string, timeout time.Duration) (*sbiTransport, error) {
	client, err := sbi.NewClient(sbi.Config{
		Services: map[string]sbi.ServiceConfig{
			smfContextServiceKey: {
				Origin:             origin.String(),
				APIRoot:            "",
				HeaderTokenProfile: smfContextTokenProfile,
			},
		},
		HeaderTokenProfiles: map[string]sbi.HeaderTokenProfile{
			smfContextTokenProfile: {
				Header: queryTokenHeader,
				Token:  queryToken,
			},
		},
		Limits: sbi.Limits{
			RequestTimeout:      timeout,
			DialTimeout:         5 * time.Second,
			MaxRequestBytes:     16 << 20,
			MaxResponseBytes:    maxAllowedResponse,
			MaxIdleConns:        16,
			MaxIdleConnsPerHost: 4,
			MaxConnsPerHost:     8,
			IdleConnTimeout:     90 * time.Second,
		},
	})
	if err != nil {
		return nil, &ConfigurationError{Reason: "shared SBI transport configuration is invalid"}
	}
	return &sbiTransport{client: client}, nil
}

func (t *sbiTransport) Do(
	ctx context.Context,
	path string,
	contentType, accept string,
	body []byte,
) (*http.Response, error) {
	response, err := t.client.Do(ctx, smfContextServiceKey, sbi.Request{
		Method:        http.MethodPost,
		Path:          path,
		ContentType:   contentType,
		Accept:        accept,
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
	})
	if err != nil {
		return nil, fmt.Errorf("OpenAMSM request failed: %w", err)
	}
	return response, nil
}

func (t *sbiTransport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}

var _ queryTransport = (*sbiTransport)(nil)
