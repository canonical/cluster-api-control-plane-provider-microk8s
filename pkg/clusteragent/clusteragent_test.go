package clusteragent

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/httptest"
)

func TestClient(t *testing.T) {
	t.Run("CanNotFindAddress", func(t *testing.T) {
		g := NewWithT(t)

		// Machines don't have any addresses.
		machines := []clusterv1.Machine{{}, {}}
		_, err := NewClient(machines, "25000", time.Second, Options{})

		g.Expect(err).To(HaveOccurred())

		// The only machine is the ignored one.
		ignoreName := "ignore"
		machines = []clusterv1.Machine{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: ignoreName,
				},
				Status: clusterv1.MachineStatus{
					Addresses: clusterv1.MachineAddresses{
						{
							Address: "1.1.1.1",
						},
					},
				},
			},
		}
		_, err = NewClient(machines, "25000", time.Second, Options{IgnoreMachineNames: sets.NewString(ignoreName)})

		g.Expect(err).To(HaveOccurred())
	})

	t.Run("CorrectEndpoint", func(t *testing.T) {
		g := NewWithT(t)

		port := "30000"
		firstAddr := "1.1.1.1"
		secondAddr := "2.2.2.2"
		thirdAddr := "3.3.3.3"

		ignoreName := "ignore"
		ignoreAddr := "8.8.8.8"
		machines := []clusterv1.Machine{
			{
				Status: clusterv1.MachineStatus{
					Addresses: clusterv1.MachineAddresses{
						{
							Address: firstAddr,
						},
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					Addresses: clusterv1.MachineAddresses{
						{
							Address: secondAddr,
						},
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					Addresses: clusterv1.MachineAddresses{
						{
							Address: thirdAddr,
						},
					},
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name: ignoreName,
				},
				Status: clusterv1.MachineStatus{
					Addresses: clusterv1.MachineAddresses{
						{
							Address: ignoreAddr,
						},
					},
				},
			},
		}

		opts := Options{
			IgnoreMachineNames: sets.NewString(ignoreName),
		}

		// NOTE(Hue): Repeat the test to make sure the ignored machine's IP is not picked by chance (reduce flakiness).
		for i := 0; i < 100; i++ {
			machines = shuffleMachines(machines)
			c, err := NewClient(machines, port, time.Second, opts)

			g.Expect(err).ToNot(HaveOccurred())

			// Check if the endpoint is one of the expected ones and not the ignored one.
			g.Expect([]string{fmt.Sprintf("https://%s:%s", firstAddr, port), fmt.Sprintf("https://%s:%s", secondAddr, port), fmt.Sprintf("https://%s:%s", thirdAddr, port)}).To(ContainElement(c.Endpoint()))
			g.Expect(c.Endpoint()).ToNot(Equal(fmt.Sprintf("https://%s:%s", ignoreAddr, port)))
		}

	})
}

func TestDo(t *testing.T) {
	g := NewWithT(t)

	path := "/random/path"
	method := http.MethodPost
	resp := map[string]string{
		"key": "value",
	}
	servM := httptest.NewServerMock(method, path, resp)
	defer servM.Srv.Close()

	ip, port, err := net.SplitHostPort(strings.TrimPrefix(servM.Srv.URL, "https://"))
	g.Expect(err).ToNot(HaveOccurred())
	c, err := NewClient([]clusterv1.Machine{
		{
			Status: clusterv1.MachineStatus{
				Addresses: clusterv1.MachineAddresses{
					{
						Address: ip,
					},
				},
			},
		},
	}, port, time.Second, Options{})

	g.Expect(err).ToNot(HaveOccurred())

	response := make(map[string]string)
	req := map[string]string{"req": "value"}
	path = strings.TrimPrefix(path, "/")
	g.Expect(c.do(context.Background(), method, path, req, nil, &response)).To(Succeed())

	g.Expect(response).To(Equal(resp))
}

func shuffleMachines(src []clusterv1.Machine) []clusterv1.Machine {
	dest := make([]clusterv1.Machine, len(src))
	perm := rand.Perm(len(src))
	for i, v := range perm {
		dest[v] = src[i]
	}
	return dest
}
