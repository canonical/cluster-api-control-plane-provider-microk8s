package clusteragent

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/images"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestClient(t *testing.T) {
	t.Run("CanNotFindAddress", func(t *testing.T) {
		g := NewWithT(t)

		// Machines don't have node refs
		machines := []clusterv1.Machine{{}, {}}
		_, err := NewClient(nil, newLogger(), machines, "25000", Options{})

		g.Expect(err).To(HaveOccurred())

		// The only machine is the ignored one.
		ignoreName := "ignore"
		machines = []clusterv1.Machine{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: ignoreName,
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "node",
					},
				},
			},
		}
		_, err = NewClient(nil, nil, machines, "25000", Options{IgnoreMachineNames: sets.NewString(ignoreName)})

		g.Expect(err).To(HaveOccurred())
	})

	t.Run("CorrectEndpoint", func(t *testing.T) {
		g := NewWithT(t)

		port := "30000"
		firstNodeName := "node-1"
		secondNodeName := "node-2"
		thirdNodeName := "node-3"

		firstAddr := "1.2.3.4"
		secondAddr := "2.3.4.5"
		thirdAddr := "3.4.5.6"

		ignoreName := "ignore"
		ignoreAddr := "9.8.7.6"
		ignoreNodeName := "node-ignore"
		machines := []clusterv1.Machine{
			{
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: firstNodeName,
					},
					Addresses: clusterv1.MachineAddresses{
						{Address: firstAddr},
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: secondNodeName,
					},
					Addresses: clusterv1.MachineAddresses{
						{Address: secondAddr},
					},
				},
			},
			{
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: thirdNodeName,
					},
					Addresses: clusterv1.MachineAddresses{
						{Address: thirdAddr},
					},
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name: ignoreName,
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: ignoreNodeName,
					},
					Addresses: clusterv1.MachineAddresses{
						{Address: ignoreAddr},
					},
				},
			},
		}

		opts := Options{
			IgnoreMachineNames: sets.NewString(ignoreName),
		}

		// NOTE(Hue): Repeat the test to make sure the ignored machine's node name is not picked by chance (reduce flakiness).
		for i := 0; i < 100; i++ {
			machines = shuffleMachines(machines)
			c, err := NewClient(nil, nil, machines, port, opts)

			g.Expect(err).ToNot(HaveOccurred())

			// Check if the endpoint is one of the expected ones and not the ignored one.
			g.Expect([]string{firstNodeName, secondNodeName, thirdNodeName}).To(ContainElement(c.nodeName))
			g.Expect([]string{firstAddr, secondAddr, thirdAddr}).To(ContainElement(c.ip))
			g.Expect(c.nodeName).ToNot(Equal(ignoreNodeName))
		}
	})
}

func TestDo(t *testing.T) {
	g := NewWithT(t)

	kubeclient := fake.NewSimpleClientset()
	nodeName := "node"
	nodeAddress := "5.6.7.8"
	port := "1234"
	method := "POST"
	endpoint := "my/endpoint"
	dataKey, dataValue := "dkey", "dvalue"
	data := map[string]any{
		dataKey: dataValue,
	}
	headerKey, headerValue := "hkey", "hvalue"
	header := map[string][]string{
		headerKey: {headerValue},
	}

	c, err := NewClient(kubeclient, newLogger(), []clusterv1.Machine{
		{
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Name: nodeName,
				},
				Addresses: clusterv1.MachineAddresses{
					{
						Address: nodeAddress,
					},
				},
			},
		},
	}, port, Options{SkipSucceededCheck: true, SkipPodCleanup: true})

	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(c.do(context.Background(), method, endpoint, header, data)).To(Succeed())

	pod, err := kubeclient.CoreV1().Pods(DefaultPodNameSpace).Get(context.Background(), fmt.Sprintf(CallerPodNameFormat, nodeName), v1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(pod.Spec.NodeName).To(Equal(nodeName))
	g.Expect(pod.Spec.Containers).To(HaveLen(1))

	container := pod.Spec.Containers[0]
	g.Expect(container.Image).To(Equal(images.CurlImage))
	g.Expect(*container.SecurityContext.Privileged).To(BeTrue())
	g.Expect(*container.SecurityContext.RunAsUser).To(Equal(int64(0)))
	g.Expect(container.Command).To(HaveLen(3))
	g.Expect(container.Command[2]).To(Equal(fmt.Sprintf(
		"curl -k -X %s -H \"%s: %s\" -d '{\"%s\":\"%s\"}' https://%s:%s/%s",
		method, headerKey, headerValue, dataKey, dataValue, nodeAddress, port, endpoint,
	)))
}

func shuffleMachines(src []clusterv1.Machine) []clusterv1.Machine {
	dest := make([]clusterv1.Machine, len(src))
	perm := rand.Perm(len(src))
	for i, v := range perm {
		dest[v] = src[i]
	}
	return dest
}

func newLogger() Logger {
	return &mockLogger{}
}

type mockLogger struct {
	entries []string
}

func (l *mockLogger) Info(msg string, keysAndValues ...interface{}) {
	l.entries = append(l.entries, msg)
}

func (l *mockLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.entries = append(l.entries, msg)
}
