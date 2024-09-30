package clusteragent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/clusteragent"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestClient(t *testing.T) {
	t.Run("CanNotFindAddress", func(t *testing.T) {
		g := NewWithT(t)

		// Machines don't have any addresses.
		machines := []clusterv1.Machine{{}, {}}
		_, err := clusteragent.NewClient(machines, "25000", time.Second, clusteragent.Options{})

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
		_, err = clusteragent.NewClient(machines, "25000", time.Second, clusteragent.Options{IgnoreMachineNames: sets.NewString(ignoreName)})

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

		opts := clusteragent.Options{
			IgnoreMachineNames: sets.NewString(ignoreName),
		}

		// NOTE(Hue): Repeat the test to make sure the ignored machine's IP is not picked by chance (reduce flakiness).
		for i := 0; i < 100; i++ {
			machines = shuffleMachines(machines)
			c, err := clusteragent.NewClient(machines, port, time.Second, opts)

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
	servM := NewServerMock(method, path, resp)
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
	}, port, time.Second, clusteragent.Options{InsecureSkipVerify: true})

	g.Expect(err).ToNot(HaveOccurred())

	response := make(map[string]string)
	req := map[string]string{"req": "value"}
	path = strings.TrimPrefix(path, "/")
	g.Expect(c.Do(context.Background(), method, path, req, &response)).To(Succeed())

	g.Expect(response).To(Equal(resp))
}

type serverMock struct {
	method   string
	path     string
	response any
	request  map[string]any
	ts       *httptest.Server
}

// NewServerMock creates a test server that responds with the given response when called with the given method and path.
// Make sure to close the server after the test is done.
// Server will try to decode the request body into a map[string]any.
func NewServerMock(method string, path string, response any) *serverMock {
	req := make(map[string]any)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if response != nil {
			if err := json.NewEncoder(w).Encode(response); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))

	return &serverMock{
		method:   method,
		path:     path,
		response: response,
		request:  req,
		ts:       ts,
	}
}

func shuffleMachines(src []clusterv1.Machine) []clusterv1.Machine {
	dest := make([]clusterv1.Machine, len(src))
	perm := rand.Perm(len(src))
	for i, v := range perm {
		dest[v] = src[i]
	}
	return dest
}
