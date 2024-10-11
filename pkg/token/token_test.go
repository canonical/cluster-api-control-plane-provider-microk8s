package token_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/canonical/cluster-api-control-plane-provider-microk8s/pkg/token"
)

func TestReconcile(t *testing.T) {
	t.Run("LookupFailsIfNoSecret", func(t *testing.T) {
		namespace := "test-namespace"
		clusterName := "test-cluster"
		c := fake.NewClientBuilder().Build()

		g := NewWithT(t)

		_, err := token.Lookup(context.Background(), c, client.ObjectKey{Name: clusterName, Namespace: namespace})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("LookupSucceedsIfSecretExists", func(t *testing.T) {
		namespace := "test-namespace"
		clusterName := "test-cluster"
		expToken := "test-token"
		secret := &corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", clusterName, token.AuthTokenNameSuffix),
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"token": []byte(expToken),
			},
		}
		c := fake.NewClientBuilder().WithObjects(secret).Build()

		g := NewWithT(t)

		token, err := token.Lookup(context.Background(), c, client.ObjectKey{Name: clusterName, Namespace: namespace})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(token).To(Equal(expToken))
	})
}
