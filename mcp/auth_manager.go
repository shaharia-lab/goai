// auth.go
package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
)

type AuthManager struct {
	config     AuthConfig
	verifier   *PKCEVerifier
	tokenStore TokenStore
}

type AuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthURL      string
	TokenURL     string
	Scopes       []string
	JWTSecret    []byte
}

type PKCEVerifier struct {
	CodeVerifier  string
	CodeChallenge string
	Method        string
}

type TokenStore interface {
	SaveToken(userID string, token *Token) error
	GetToken(userID string) (*Token, error)
	RevokeToken(userID string) error
}

type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewAuthManager(config AuthConfig, tokenStore TokenStore) *AuthManager {
	return &AuthManager{
		config:     config,
		tokenStore: tokenStore,
	}
}

func (am *AuthManager) GenerateAuthURL() (string, error) {
	verifier, err := am.generatePKCEVerifier()
	if err != nil {
		return "", fmt.Errorf("failed to generate PKCE verifier: %w", err)
	}

	am.verifier = verifier

	params := map[string]string{
		"response_type":         "code",
		"client_id":             am.config.ClientID,
		"redirect_uri":          am.config.RedirectURI,
		"scope":                 strings.Join(am.config.Scopes, " "),
		"code_challenge":        verifier.CodeChallenge,
		"code_challenge_method": verifier.Method,
		"state":                 generateState(),
	}

	return buildURL(am.config.AuthURL, params), nil
}

func (am *AuthManager) HandleCallback(code string) (*Token, error) {
	if am.verifier == nil {
		return nil, fmt.Errorf("PKCE verifier not initialized")
	}

	token, err := am.exchangeCode(code, am.verifier.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %w", err)
	}

	return token, nil
}

func (am *AuthManager) ValidateToken(tokenString string) (*jwt.StandardClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.config.JWTSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if claims, ok := token.Claims.(*jwt.StandardClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

func (am *AuthManager) generatePKCEVerifier() (*PKCEVerifier, error) {
	verifier := make([]byte, 32)
	if _, err := rand.Read(verifier); err != nil {
		return nil, err
	}

	verifierStr := base64.RawURLEncoding.EncodeToString(verifier)
	challenge := generatePKCEChallenge(verifierStr)

	return &PKCEVerifier{
		CodeVerifier:  verifierStr,
		CodeChallenge: challenge,
		Method:        "S256",
	}, nil
}

func (am *AuthManager) exchangeCode(code, codeVerifier string) (*Token, error) {
	// Implementation of OAuth 2.1 code exchange
	// This would make an HTTP request to the token endpoint
	// and handle the response
	return nil, nil
}

// Generate a random state parameter for OAuth flow
func generateState() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Build URL with query parameters
func buildURL(baseURL string, params map[string]string) string {
	values := make([]string, 0, len(params))
	for key, value := range params {
		values = append(values, fmt.Sprintf("%s=%s", key, url.QueryEscape(value)))
	}
	if len(values) > 0 {
		return fmt.Sprintf("%s?%s", baseURL, strings.Join(values, "&"))
	}
	return baseURL
}

// Generate PKCE challenge from code verifier
func generatePKCEChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
