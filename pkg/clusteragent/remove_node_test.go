package clusteragent_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/clusteragent"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestRemoveFromDqlite(t *testing.T) {
	g := NewWithT(t)

	path := "/cluster/api/v2.0/dqlite/remove"
	method := http.MethodPost
	servM := NewServerMock(method, path, nil)
	defer servM.ts.Close()

	ip, port, err := net.SplitHostPort(strings.TrimPrefix(servM.ts.URL, "https://"))
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
	}, clusteragent.Options{Port: port, InsecureSkipVerify: true})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.RemoveNodeFromDqlite(context.Background(), "1.1.1.1:1234")).To(Succeed())
	g.Expect(servM.request).To(HaveKeyWithValue("removeEndpoint", "1.1.1.1:1234"))
}
