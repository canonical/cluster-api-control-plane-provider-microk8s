package token

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AuthTokenNameSuffix = "capi-auth-token"
)

// Lookup retrieves the token for the given cluster.
func Lookup(ctx context.Context, c client.Client, clusterKey client.ObjectKey) (string, error) {
	secret, err := getSecret(ctx, c, clusterKey)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	v, ok := secret.Data["token"]
	if !ok {
		return "", fmt.Errorf("token not found in secret")
	}

	return string(v), nil
}

// getSecret retrieves the token secret for the given cluster.
func getSecret(ctx context.Context, c client.Client, clusterKey client.ObjectKey) (*corev1.Secret, error) {
	s := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      authTokenName(clusterKey.Name),
		Namespace: clusterKey.Namespace,
	}
	if err := c.Get(ctx, key, s); err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	return s, nil
}

// authTokenName returns the name of the auth-token secret, computed by convention using the name of the cluster.
func authTokenName(clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, AuthTokenNameSuffix)
}
