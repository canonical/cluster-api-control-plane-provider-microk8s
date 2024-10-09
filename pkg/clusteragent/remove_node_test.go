package clusteragent_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/clusteragent"
	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/httptest"
)

func TestRemoveFromDqlite(t *testing.T) {
	g := NewWithT(t)

	path := "/cluster/api/v2.0/dqlite/remove"
	method := http.MethodPost
	servM := httptest.NewServerMock(method, path, nil)
	defer servM.Srv.Close()

	ip, port, err := net.SplitHostPort(strings.TrimPrefix(servM.Srv.URL, "https://"))
	g.Expect(err).ToNot(HaveOccurred())
	c, err := clusteragent.NewClient([]clusterv1.Machine{
		{
			Status: clusterv1.MachineStatus{
				Addresses: clusterv1.MachineAddresses{
					{
						Address: ip,
					},
				},
			},
		},
	}, port, time.Second, clusteragent.Options{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.RemoveNodeFromDqlite(context.Background(), "1.1.1.1:1234")).To(Succeed())
	g.Expect(servM.Request).To(HaveKeyWithValue("removeEndpoint", "1.1.1.1:1234"))
	// // TODO(Hue): change the token
	// g.Expect(servM.Header.Get("callback_token")).To(Equal("myRandomToken"))
}
