package clusteragent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Options should be used when initializing a new client.
type Options struct {
	// IgnoreMachineNames is a set of ignored machine names that we don't want to pick their IP for the cluster agent endpoint.
	IgnoreMachineNames sets.String
}

type Client struct {
	ip, port string
	client   *http.Client
}

// NewClient picks an IP from one of the given machines and creates a new client for the cluster agent
// with that IP.
func NewClient(machines []clusterv1.Machine, port string, timeout time.Duration, opts Options) (*Client, error) {
	var ip string
	for _, m := range machines {
		if opts.IgnoreMachineNames.Has(m.Name) {
			continue
		}

		for _, addr := range m.Status.Addresses {
			if net.ParseIP(addr.Address) != nil {
				ip = addr.Address
				break
			}
		}
		break
	}

	if ip == "" {
		return nil, errors.New("failed to find an IP for cluster agent")
	}

	return &Client{
		ip:   ip,
		port: port,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					// TODO(Hue): Workaround for now, address later on
					// get the certificate fingerprint from the matching node through a resource in the cluster (TBD),
					//  and validate it in the TLSClientConfig
					InsecureSkipVerify: true,
				},
			},
		},
	}, nil
}

func (c *Client) Endpoint() string {
	return fmt.Sprintf("https://%s:%s", c.ip, c.port)
}

// do makes a request to the given endpoint with the given method. It marshals the request and unmarshals
// server response body if the provided response is not nil.
// The endpoint should _not_ have a leading slash.
func (c *Client) do(ctx context.Context, method, endpoint string, request any, header map[string][]string, response any) error {
	url := fmt.Sprintf("https://%s:%s/%s", c.ip, c.port, endpoint)

	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to prepare worker info request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header = http.Header(header)

	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call cluster agent: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// NOTE(hue): Marshal and print any response that we got since it might contain valuable information
		// on why the request failed.
		// Ignore JSON errors to prevent unnecessarily complicated error handling.
		anyResp := make(map[string]any)
		_ = json.NewDecoder(res.Body).Decode(&anyResp)
		b, _ := json.Marshal(anyResp)
		resStr := string(b)

		return fmt.Errorf("HTTP request to cluster agent failed with status code %d, got response: %q", res.StatusCode, resStr)
	}

	if response != nil {
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
