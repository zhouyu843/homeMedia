package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName   = "homemedia_session"
	loginCSRFCookieName = "homemedia_login_csrf"
)

type AuthService struct {
	adminUsername string
	adminPassword string
	sessionSecret []byte
	sessionTTL    time.Duration
}

func NewAuthService(adminUsername string, adminPassword string, sessionSecret string, sessionTTL time.Duration) *AuthService {
	return &AuthService{
		adminUsername: adminUsername,
		adminPassword: adminPassword,
		sessionSecret: []byte(sessionSecret),
		sessionTTL:    sessionTTL,
	}
}

func (a *AuthService) IsAuthenticated(r *http.Request) bool {
	_, ok := a.currentUser(r)
	return ok
}

func (a *AuthService) AuthenticateCredentials(username string, password string) bool {
	if subtle.ConstantTimeCompare([]byte(username), []byte(a.adminUsername)) != 1 {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(password), []byte(a.adminPassword)) == 1
}

func (a *AuthService) StartSession(c *gin.Context, username string) string {
	expiresAt := time.Now().UTC().Add(a.sessionTTL).Unix()
	payload := username + "|" + strconv.FormatInt(expiresAt, 10)
	sig := a.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig)

	maxAge := int(a.sessionTTL.Seconds())
	c.SetCookie(sessionCookieName, value, maxAge, "/", "", false, true)
	return a.sessionCSRFTokenFromValue(value)
}

func (a *AuthService) EndSession(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func (a *AuthService) SessionCSRFToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}

	return a.sessionCSRFTokenFromValue(cookie.Value), true
}

func (a *AuthService) sessionCSRFTokenFromValue(cookieValue string) string {
	mac := hmac.New(sha256.New, a.sessionSecret)
	_, _ = mac.Write([]byte("csrf|" + cookieValue))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *AuthService) VerifySessionCSRF(r *http.Request, token string) bool {
	expected, ok := a.SessionCSRFToken(r)
	if !ok {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (a *AuthService) IssueLoginCSRF(c *gin.Context) (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(random)
	c.SetCookie(loginCSRFCookieName, token, 600, "/", "", false, true)
	return token, nil
}

func (a *AuthService) VerifyLoginCSRF(r *http.Request, token string) bool {
	if token == "" {
		return false
	}

	cookie, err := r.Cookie(loginCSRFCookieName)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(cookie.Value)) == 1
}

func (a *AuthService) ClearLoginCSRF(c *gin.Context) {
	c.SetCookie(loginCSRFCookieName, "", -1, "/", "", false, true)
}

func (a *AuthService) currentUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return "", false
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}

	sigRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}

	expectedSig := a.sign(string(payloadRaw))
	if subtle.ConstantTimeCompare(sigRaw, expectedSig) != 1 {
		return "", false
	}

	payloadParts := strings.Split(string(payloadRaw), "|")
	if len(payloadParts) != 2 {
		return "", false
	}

	expiresAt, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil {
		return "", false
	}

	if time.Now().UTC().After(time.Unix(expiresAt, 0).UTC()) {
		return "", false
	}

	return payloadParts[0], true
}

func (a *AuthService) CurrentUser(r *http.Request) (string, bool) {
	return a.currentUser(r)
}

func (a *AuthService) sign(payload string) []byte {
	mac := hmac.New(sha256.New, a.sessionSecret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func (a *AuthService) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.IsAuthenticated(c.Request) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "authentication required"})
				c.Abort()
				return
			}

			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
		c.Next()
	}
}
