package vault

import (
	"context"
	"fmt"

	vapi "github.com/hashicorp/vault/api"
)

// unwrap uses the sys/unwrap endpoint via the SDK.
func unwrap(ctx context.Context, c *vapi.Client, wrappedToken string) (string, error) {
	// For unwrap, the wrapped token must be set as the client token
	orig := c.Token()
	c.SetToken(wrappedToken)
	defer c.SetToken(orig)

	sec, err := c.Logical().Unwrap("")
	if err != nil {
		return "", fmt.Errorf("vault unwrap: %w", err)
	}
	if sec == nil || sec.Data == nil {
		return "", fmt.Errorf("vault unwrap: empty response")
	}
	// Common key
	if v, ok := sec.Data["secret_id"].(string); ok && v != "" {
		return v, nil
	}
	// Fallback: first string field
	for _, val := range sec.Data {
		if s, ok := val.(string); ok && s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("vault unwrap: secret_id not found")
}

// approleLogin performs AppRole login and returns the auth secret used for renewal.
func approleLogin(ctx context.Context, c *vapi.Client, roleID, secretID string) (*vapi.Secret, error) {
	data := map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	}
	sec, err := c.Logical().WriteWithContext(ctx, "auth/approle/login", data)
	if err != nil {
		return nil, fmt.Errorf("vault approle login: %w", err)
	}
	if sec == nil || sec.Auth == nil || sec.Auth.ClientToken == "" {
		return nil, fmt.Errorf("vault approle login: empty auth token")
	}
	// Set client token for subsequent calls
	c.SetToken(sec.Auth.ClientToken)
	return sec, nil
}

// startRenewer starts a background renewer on the provided secret.
// if your AppRole is issuing batch tokens (non-renewable), DoneCh() will fire when it expires; the fix is to configure AppRole to issue renewable service tokens or handle the re-login path when DoneCh() triggers. (That behavior is a known gotcha discussed in the community.)
func startRenewer(ctx context.Context, c *vapi.Client, auth *vapi.Secret) func() error {
	renewer, err := c.NewLifetimeWatcher(&vapi.LifetimeWatcherInput{Secret: auth})
	if err != nil {
		// No renewer; nothing to stop
		return func() error { return nil }
	}
	// run renewer
	go renewer.Renew()
	// watch channels
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case renew := <-renewer.RenewCh():
				if renew.Secret != nil && renew.Secret.Auth != nil && len(renew.Secret.Auth.ClientToken) > 0 {
					c.SetToken(renew.Secret.Auth.ClientToken)
				}
				return
			case err = <-renewer.DoneCh():
				if err != nil {
					_ = err // could log it
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	// return closer
	return func() error {
		renewer.Stop()
		<-done
		return nil
	}
}
