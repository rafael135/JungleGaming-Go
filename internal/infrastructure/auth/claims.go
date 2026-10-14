package auth

import "context"

type contextKey string

const claimsContextKey = contextKey("auth_claims")

type AuthClaims struct {
	Subject    string   `json:"sub"`
	ClientID   string   `json:"clientId"`
	ProviderID string   `json:"providerId"`
	Roles      []string `json:"roles"`
}

func (c *AuthClaims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func ContextWithClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func ClaimsFromContext(ctx context.Context) (*AuthClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*AuthClaims)
	return claims, ok && claims != nil
}
