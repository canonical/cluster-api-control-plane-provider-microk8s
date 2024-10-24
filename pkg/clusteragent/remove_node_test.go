package clusteragent_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/clusteragent"
)

func TestRemoveFromDqlite(t *testing.T) {
	g := NewWithT(t)

	token := "token"
	port := "1234"
	removeEp := "1.1.1.1:9876"
	machineIP := "5.6.7.8"
	nodeName := "node-1"
	kubeclient := fake.NewSimpleClientset()
	c, err := clusteragent.NewClient(kubeclient, newLogger(), []clusterv1.Machine{
		{
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Name: nodeName,
				},
				Addresses: clusterv1.MachineAddresses{
					{
						Address: machineIP,
					},
				},
			},
		},
	}, port, clusteragent.Options{SkipSucceededCheck: true, SkipPodCleanup: true})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.RemoveNodeFromDqlite(context.Background(), token, removeEp)).To(Succeed())

	pod, err := kubeclient.CoreV1().Pods(clusteragent.DefaultPodNameSpace).Get(context.Background(), fmt.Sprintf(clusteragent.CallerPodNameFormat, nodeName), v1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pod.Spec.Containers).To(HaveLen(1))

	container := pod.Spec.Containers[0]
	g.Expect(container.Command).To(HaveLen(3))
	g.Expect(container.Command[2]).To(Equal(fmt.Sprintf("curl -k -X POST -H \"capi-auth-token: %s\" -d '{\"remove_endpoint\":\"%s\"}' https://%s:%s/cluster/api/v2.0/dqlite/remove", token, removeEp, machineIP, port)))
}

func newLogger() clusteragent.Logger {
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
