package controllers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"time"

	bootstrapv1beta1 "github.com/canonical/cluster-api-bootstrap-provider-microk8s/apis/v1beta1"

	clusterv1beta1 "github.com/canonical/cluster-api-control-plane-provider-microk8s/api/v1beta1"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/connrotation"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	templateConfig string = `
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: <CACERT>
    server: https://<HOST>:6443
  name: microk8s-cluster
contexts:
- context:
    cluster: microk8s-cluster
    user: admin
  name: microk8s
current-context: microk8s
kind: Config
preferences: {}
users:
- name: admin
  user:
    client-certificate-data: <CERT>
    client-key-data: <KEY>
`
)

type kubernetesClient struct {
	*kubernetes.Clientset

	dialer *connrotation.Dialer
}

// Close kubernetes client.
func (k *kubernetesClient) Close() error {
	k.dialer.CloseAll()

	return nil
}

func newDialer() *connrotation.Dialer {
	return connrotation.NewDialer((&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext)
}

// kubeconfigForCluster will fetch a kubeconfig secret based on cluster name/namespace,
// use it to create a clientset, and return it.
func (r *MicroK8sControlPlaneReconciler) kubeconfigForCluster(ctx context.Context, cluster client.ObjectKey) (*kubernetesClient, error) {
	kubeconfigSecret := &corev1.Secret{}

	// See if the kubeconfig exists. If not create it.
	secrets := &corev1.SecretList{}
	err := r.Client.List(ctx, secrets)
	if err != nil {
		return nil, err
	}

	found := false
	for _, s := range secrets.Items {
		if s.Name == cluster.Name+"-kubeconfig" {
			found = true
		}
	}

	c := &clusterv1.Cluster{}
	err = r.Client.Get(ctx, cluster, c)
	if err != nil {
		return nil, err
	}
	if !found && c.Spec.ControlPlaneEndpoint.IsValid() {
		kubeconfig, err := r.genarateKubeconfig(ctx, cluster, c.Spec.ControlPlaneEndpoint.Host)
		if err != nil {
			return nil, err
		}
		configsecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      cluster.Name + "-kubeconfig",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
			Data: map[string][]byte{
				"value": []byte(*kubeconfig),
			},
		}
		err = r.Client.Create(ctx, configsecret)
		if err != nil {
			return nil, err
		}
	}

	err = r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name + "-kubeconfig",
		},
		kubeconfigSecret,
	)
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigSecret.Data["value"])
	if err != nil {
		return nil, err
	}

	dialer := newDialer()
	config.Dial = dialer.DialContext

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &kubernetesClient{
		Clientset: clientset,
		dialer:    dialer,
	}, nil
}

func (r *MicroK8sControlPlaneReconciler) genarateKubeconfig(ctx context.Context, cluster client.ObjectKey, host string) (kubeconfig *string, err error) {
	// Get the secret with the CA
	readCASecret := &corev1.Secret{}
	err = r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name + "-ca",
		},
		readCASecret,
	)
	if err != nil {
		return nil, err
	}

	// Decode certificates
	caCertBlock, _ := pem.Decode(readCASecret.Data["crt"])
	if caCertBlock == nil || caCertBlock.Type != "CERTIFICATE" {
		return nil, errors.New("failed to decode CA certificate")
	}

	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(readCASecret.Data["key"])
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, errors.New("failed to decode CA key")
	}

	caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	// set up client certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization:  []string{"admin", "system:masters"},
			Country:       []string{"GB"},
			Province:      []string{""},
			Locality:      []string{"Canonical"},
			StreetAddress: []string{"Canonical"},
			CommonName:    "admin",
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, &certPrivKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}); err != nil {
		return nil, err
	}

	keyPEM := new(bytes.Buffer)
	if err := pem.Encode(keyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	}); err != nil {
		return nil, err
	}

	config := strings.Replace(templateConfig, "<HOST>", host, -1)
	config = strings.Replace(config, "<CACERT>", base64.StdEncoding.EncodeToString(readCASecret.Data["crt"]), -1)
	config = strings.Replace(config, "<CERT>", base64.StdEncoding.EncodeToString(certPEM.Bytes()), -1)
	config = strings.Replace(config, "<KEY>", base64.StdEncoding.EncodeToString(keyPEM.Bytes()), -1)

	return &config, nil
}

func (r *MicroK8sControlPlaneReconciler) generateMicroK8sConfig(ctx context.Context, tcp *clusterv1beta1.MicroK8sControlPlane,
	cluster *clusterv1.Cluster, spec *bootstrapv1beta1.MicroK8sConfigSpec) (*corev1.ObjectReference, error) {
	owner := metav1.OwnerReference{
		APIVersion:         clusterv1beta1.GroupVersion.String(),
		Kind:               "MicroK8sControlPlane",
		Name:               tcp.Name,
		UID:                tcp.UID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}

	bootstrapConfig := &bootstrapv1beta1.MicroK8sConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(tcp.Name + "-"),
			Namespace:       tcp.Namespace,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1beta1.GroupVersion.String(),
		Kind:       "MicroK8sConfig",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}
