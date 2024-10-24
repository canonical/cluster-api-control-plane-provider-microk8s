package clusteragent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/control"
	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/images"
)

const (
	CallerPodNameFormat string = "cluster-agent-caller-%s"
	DefaultPodNameSpace string = "default"
)

// Options should be used when initializing a new client.
type Options struct {
	// IgnoreMachineNames is a set of ignored machine names that we don't want to pick their node name for the cluster agent.
	IgnoreMachineNames sets.String
	// SkipSucceededCheck skips the waiting for succeeded phase on pod. Mostly used for testing purposes.
	SkipSucceededCheck bool
	// SkipPodCleanup skips the pod cleanup after the request is done. Mostly used for testing purposes.
	SkipPodCleanup bool
}

// KubeClient is an interface for the Kubernetes client.
type KubeClient interface {
	CoreV1() corev1client.CoreV1Interface
}

// Client is a client for the cluster agent.
type Client struct {
	KubeClient
	logger    Logger
	nodeName  string
	namespace string
	ip        string
	port      string

	skipSucceededCheck bool
	skipPodCleanup     bool
}

// Logger is an interface for logging.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(err error, msg string, keysAndValues ...interface{})
}

// NewClient picks a node name and IP from one of the given machines and creates a new client for the cluster agent.
func NewClient(kubeclient KubeClient, logger Logger, machines []clusterv1.Machine, port string, opts Options) (*Client, error) {
	var nodeName string
	var ip string
	for _, m := range machines {
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		if opts.IgnoreMachineNames.Has(m.Name) {
			continue
		}

		if m.Status.NodeRef != nil {
			nodeName = m.Status.NodeRef.Name
			for _, addr := range m.Status.Addresses {
				if net.ParseIP(addr.Address) != nil {
					ip = addr.Address
					break
				}
			}
			break
		}
	}

	if nodeName == "" {
		return nil, errors.New("failed to find a node for cluster agent")
	}

	if ip == "" {
		return nil, errors.New("failed to find an IP address for cluster agent")
	}

	return &Client{
		KubeClient:         kubeclient,
		logger:             logger,
		nodeName:           nodeName,
		namespace:          DefaultPodNameSpace,
		ip:                 ip,
		port:               port,
		skipSucceededCheck: opts.SkipSucceededCheck,
		skipPodCleanup:     opts.SkipPodCleanup,
	}, nil
}

// do sends a request to the cluster agent.
func (c *Client) do(ctx context.Context, method, endpoint string, header http.Header, data map[string]any) error {
	pod, err := c.createPod(ctx, method, endpoint, header, data)
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}

	if !c.skipPodCleanup {
		defer func() {
			if err := c.deletePod(ctx, pod.Name); err != nil {
				c.logger.Error(err, "failed to delete pod")
			}
		}()
	}

	if c.skipSucceededCheck {
		return nil
	}

	podName := pod.Name
	if err := control.WaitUntilReady(ctx, func() (bool, error) {
		pod, err := c.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get pod: %w", err)
		}

		if pod.Status.Phase == corev1.PodSucceeded {
			return true, nil
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("pod failed")
		}

		return false, nil
	}, control.WaitOptions{NumRetries: ptr.To(120)}); err != nil {
		return fmt.Errorf("failed to wait for pod to succeed: %w", err)
	}

	return nil
}

// createPod creates a pod that runs a curl command.
func (c *Client) createPod(ctx context.Context, method, endpoint string, header http.Header, data map[string]any) (*corev1.Pod, error) {
	curl, err := c.createCURLString(method, endpoint, header, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create curl string: %w", err)
	}

	c.logger.Info("creating curl pod", "cmd", curl)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf(CallerPodNameFormat, c.nodeName),
		},
		Spec: corev1.PodSpec{
			NodeName: c.nodeName,
			Containers: []corev1.Container{
				{
					Name:  "caller",
					Image: images.CurlImage,
					Command: []string{
						"su",
						"-c",
						curl,
					},
					SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true), RunAsUser: ptr.To(int64(0))},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	pod, err = c.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	return pod, nil
}

// createCURLString creates a curl command string.
func (c *Client) createCURLString(method, endpoint string, header http.Header, data map[string]any) (string, error) {
	// Method
	req := fmt.Sprintf("curl -k -X %s", method)

	// Headers
	for h, vv := range header {
		for _, v := range vv {
			req += fmt.Sprintf(" -H \"%s: %s\"", h, v)
		}
	}

	// Data
	if data != nil {
		dataB, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("failed to marshal data: %w", err)
		}
		req += fmt.Sprintf(" -d '%s'", string(dataB))
	}

	// Endpoint
	req += fmt.Sprintf(" https://%s:%s/%s", c.ip, c.port, endpoint)

	return req, nil
}

// deletePod deletes a pod.
func (c *Client) deletePod(ctx context.Context, podName string) error {
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	}

	if err := control.WaitUntilReady(ctx, func() (bool, error) {
		err := c.CoreV1().Pods(c.namespace).Delete(ctx, podName, deleteOptions)
		if err != nil && !apierrors.IsNotFound(err) {
			return false, nil
		}
		return true, nil
	}, control.WaitOptions{NumRetries: ptr.To(120)}); err != nil {
		return fmt.Errorf("failed to wait for pod deletion: %w", err)
	}

	return nil
}
