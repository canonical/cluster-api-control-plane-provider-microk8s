package clusteragent

import (
	"context"
	"net/http"
)

// RemoveNodeFromDqlite calls the /v2/dqlite/remove endpoint on cluster agent to remove the given address from Dqlite.
// The endpoint should be in the format of "address:port".
func (p *Client) RemoveNodeFromDqlite(ctx context.Context, removeEp string) error {
	request := map[string]string{"removeEndpoint": removeEp}
	return p.Do(ctx, http.MethodPost, "cluster/api/v2.0/dqlite/remove", request, nil)
}
