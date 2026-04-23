package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type googleClaims struct {
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Aud           string `json:"aud"`
	Error         string `json:"error"`
}

// verifyGoogleToken validates a Google ID token via the tokeninfo endpoint.
// Returns the verified email and display name, or an error.
func verifyGoogleToken(idToken, clientID string) (email, name string, err error) {
	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + idToken)
	if err != nil {
		return "", "", fmt.Errorf("google: request failed: %w", err)
	}
	defer resp.Body.Close()

	var claims googleClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return "", "", fmt.Errorf("google: decode failed: %w", err)
	}
	if claims.Error != "" {
		return "", "", fmt.Errorf("google: invalid token: %s", claims.Error)
	}
	if claims.EmailVerified != "true" {
		return "", "", fmt.Errorf("google: email not verified")
	}
	if clientID != "" && claims.Aud != clientID {
		return "", "", fmt.Errorf("google: token audience mismatch")
	}
	return claims.Email, claims.Name, nil
}

// bearerToken extracts the token from "Authorization: Bearer <token>".
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return ""
}
