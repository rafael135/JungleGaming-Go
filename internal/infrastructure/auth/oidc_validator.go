package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrMissingAuthHeader = errors.New("missing or malformed authorization header")
	ErrInvalidToken      = errors.New("invalid or expired token")
	ErrKeyNotFound       = errors.New("jwks key not found")
	ErrForbidden         = errors.New("access forbidden")
)

type TokenValidator interface {
	ValidateToken(ctx context.Context, tokenStr string) (*AuthClaims, error)
}

type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSResponse struct {
	Keys []JWK `json:"keys"`
}

type KeycloakOIDCValidator struct {
	issuerURL  string
	jwksURL    string
	httpClient *http.Client

	mu        sync.RWMutex
	keysCache map[string]*rsa.PublicKey
	cachedAt  time.Time
	cacheTTL  time.Duration
}

func NewKeycloakOIDCValidator(issuerURL string, httpClient *http.Client) *KeycloakOIDCValidator {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	jwksURL := strings.TrimSuffix(issuerURL, "/") + "/protocol/openid-connect/certs"

	return &KeycloakOIDCValidator{
		issuerURL:  issuerURL,
		jwksURL:    jwksURL,
		httpClient: httpClient,
		keysCache:  make(map[string]*rsa.PublicKey),
		cacheTTL:   1 * time.Hour,
	}
}

func (v *KeycloakOIDCValidator) ValidateToken(ctx context.Context, tokenStr string) (*AuthClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("metodo de assinatura inesperado: %v", t.Header["alg"])
		}

		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, errors.New("token header sem kid")
		}

		pubKey, err := v.getKey(ctx, kid)
		if err != nil {
			return nil, err
		}

		return pubKey, nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claimsMap, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	claims := &AuthClaims{}

	if sub, ok := claimsMap["sub"].(string); ok {
		claims.Subject = sub
	}

	if clientID, ok := claimsMap["client_id"].(string); ok {
		claims.ClientID = clientID
	} else if clientID, ok := claimsMap["clientId"].(string); ok {
		claims.ClientID = clientID
	} else if clientID, ok := claimsMap["azp"].(string); ok {
		claims.ClientID = clientID
	}

	if pID, ok := claimsMap["provider_id"].(string); ok {
		claims.ProviderID = pID
	} else if pID, ok := claimsMap["providerId"].(string); ok {
		claims.ProviderID = pID
	} else {
		if strings.HasPrefix(claims.ClientID, "provider-") {
			claims.ProviderID = claims.ClientID
		}
	}

	if realmAccess, ok := claimsMap["realm_access"].(map[string]interface{}); ok {
		if rolesRaw, ok := realmAccess["roles"].([]interface{}); ok {
			for _, r := range rolesRaw {
				if rStr, ok := r.(string); ok {
					claims.Roles = append(claims.Roles, rStr)
				}
			}
		}
	}

	if len(claims.Roles) == 0 {
		if resAccess, ok := claimsMap["resource_access"].(map[string]interface{}); ok {
			if clientAccess, ok := resAccess[claims.ClientID].(map[string]interface{}); ok {
				if rolesRaw, ok := clientAccess["roles"].([]interface{}); ok {
					for _, r := range rolesRaw {
						if rStr, ok := r.(string); ok {
							claims.Roles = append(claims.Roles, rStr)
						}
					}
				}
			}
		}
	}

	return claims, nil
}

func (v *KeycloakOIDCValidator) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, exists := v.keysCache[kid]
	fresh := time.Since(v.cachedAt) < v.cacheTTL
	v.mu.RUnlock()

	if exists && fresh {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		if exists {
			return key, nil
		}
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	key, exists = v.keysCache[kid]
	if !exists {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

func (v *KeycloakOIDCValidator) refreshKeys(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao buscar JWKS do Keycloak: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resposta inesperada do JWKS: status %d", resp.StatusCode)
	}

	var jwks JWKSResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("erro ao decodificar JWKS JSON: %w", err)
	}

	newCache := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}

		pubKey, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}

		newCache[k.Kid] = pubKey
	}

	v.keysCache = newCache
	v.cachedAt = time.Now()

	return nil
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := decodeBase64URL(nStr)
	if err != nil {
		return nil, fmt.Errorf("n invalido: %w", err)
	}

	eBytes, err := decodeBase64URL(eStr)
	if err != nil {
		return nil, fmt.Errorf("e invalido: %w", err)
	}

	var eInt int
	for _, b := range eBytes {
		eInt = (eInt << 8) | int(b)
	}

	pubKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eInt,
	}

	return pubKey, nil
}

func decodeBase64URL(s string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return data, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
