package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
)

type contextKey string

const UserKey contextKey = "auth_user"

// Provider identifiers for SSO.
const (
	ProviderGoogle    = "google"
	ProviderMicrosoft = "microsoft"
)

// User is the authenticated principal after SSO.
type User struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
}

// ProviderConfig holds OAuth client settings for one IdP.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	Tenant       string // Microsoft only; default "common"
}

// Service issues sessions after Google/Microsoft OAuth.
type Service struct {
	Secret      []byte
	TokenTTL    time.Duration
	Bypass      bool // local/dev when no IdP configured
	WebOrigin   string
	APIOrigin   string
	Google      ProviderConfig
	Microsoft   ProviderConfig
	HTTPClient  *http.Client
}

type tokenPayload struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
	Exp      int64  `json:"exp"`
}

type oauthState struct {
	Nonce    string `json:"n"`
	Provider string `json:"p"`
	Exp      int64  `json:"e"`
}

func New(secret string, bypass bool, webOrigin, apiOrigin string, googleCfg, msCfg ProviderConfig) *Service {
	if secret == "" {
		secret = "docforge-dev-secret-change-me"
	}
	if webOrigin == "" {
		webOrigin = "http://localhost:5173"
	}
	if apiOrigin == "" {
		apiOrigin = "http://localhost:8080"
	}
	if msCfg.Tenant == "" {
		msCfg.Tenant = "common"
	}
	return &Service{
		Secret:     []byte(secret),
		TokenTTL:   24 * time.Hour,
		Bypass:     bypass,
		WebOrigin:  strings.TrimRight(webOrigin, "/"),
		APIOrigin:  strings.TrimRight(apiOrigin, "/"),
		Google:     googleCfg,
		Microsoft:  msCfg,
		HTTPClient: http.DefaultClient,
	}
}

func (s *Service) EnabledProviders() []map[string]string {
	out := make([]map[string]string, 0, 2)
	if s.Google.ClientID != "" && s.Google.ClientSecret != "" {
		out = append(out, map[string]string{
			"id":    ProviderGoogle,
			"name":  "Google",
			"login": "/api/v1/auth/google/login",
		})
	}
	if s.Microsoft.ClientID != "" && s.Microsoft.ClientSecret != "" {
		out = append(out, map[string]string{
			"id":    ProviderMicrosoft,
			"name":  "Microsoft",
			"login": "/api/v1/auth/microsoft/login",
		})
	}
	return out
}

func (s *Service) oauthConfig(provider string) (*oauth2.Config, error) {
	switch provider {
	case ProviderGoogle:
		if s.Google.ClientID == "" || s.Google.ClientSecret == "" {
			return nil, domain.NewAppError(domain.CodeUnauthorized, "google SSO is not configured", false)
		}
		return &oauth2.Config{
			ClientID:     s.Google.ClientID,
			ClientSecret: s.Google.ClientSecret,
			RedirectURL:  s.APIOrigin + "/api/v1/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}, nil
	case ProviderMicrosoft:
		if s.Microsoft.ClientID == "" || s.Microsoft.ClientSecret == "" {
			return nil, domain.NewAppError(domain.CodeUnauthorized, "microsoft SSO is not configured", false)
		}
		tenant := s.Microsoft.Tenant
		return &oauth2.Config{
			ClientID:     s.Microsoft.ClientID,
			ClientSecret: s.Microsoft.ClientSecret,
			RedirectURL:  s.APIOrigin + "/api/v1/auth/microsoft/callback",
			Scopes:       []string{"openid", "email", "profile", "User.Read"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenant),
				TokenURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant),
			},
		}, nil
	default:
		return nil, domain.NewAppError(domain.CodeUnauthorized, "unknown SSO provider", false)
	}
}

// AuthCodeURL builds the IdP redirect URL with a signed state.
func (s *Service) AuthCodeURL(provider string) (string, error) {
	cfg, err := s.oauthConfig(provider)
	if err != nil {
		return "", err
	}
	state, err := s.signState(provider)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("prompt", "select_account")), nil
}

func (s *Service) signState(provider string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := oauthState{
		Nonce:    base64.RawURLEncoding.EncodeToString(nonce),
		Provider: provider,
		Exp:      time.Now().Add(10 * time.Minute).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func (s *Service) verifyState(provider, state string) error {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return domain.NewAppError(domain.CodeUnauthorized, "invalid oauth state", false)
	}
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return domain.NewAppError(domain.CodeUnauthorized, "invalid oauth state", false)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return domain.NewAppError(domain.CodeUnauthorized, "invalid oauth state", false)
	}
	var st oauthState
	if err := json.Unmarshal(raw, &st); err != nil {
		return domain.NewAppError(domain.CodeUnauthorized, "invalid oauth state", false)
	}
	if st.Provider != provider || time.Now().Unix() > st.Exp {
		return domain.NewAppError(domain.CodeUnauthorized, "oauth state expired", false)
	}
	return nil
}

// HandleCallback exchanges the code and returns a DocForge session token + user.
func (s *Service) HandleCallback(ctx context.Context, provider, code, state string) (token string, user User, err error) {
	if err := s.verifyState(provider, state); err != nil {
		return "", User{}, err
	}
	cfg, err := s.oauthConfig(provider)
	if err != nil {
		return "", User{}, err
	}
	tok, err := cfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, s.HTTPClient), code)
	if err != nil {
		return "", User{}, domain.NewAppError(domain.CodeUnauthorized, "oauth token exchange failed", true)
	}
	user, err = s.fetchUser(ctx, provider, tok)
	if err != nil {
		return "", User{}, err
	}
	token, err = s.issue(user)
	return token, user, err
}

func (s *Service) fetchUser(ctx context.Context, provider string, tok *oauth2.Token) (User, error) {
	client := oauth2.NewClient(context.WithValue(ctx, oauth2.HTTPClient, s.HTTPClient), oauth2.StaticTokenSource(tok))
	switch provider {
	case ProviderGoogle:
		resp, err := client.Get("https://openidconnect.googleapis.com/v1/userinfo")
		if err != nil {
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "failed to fetch google profile", true)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "google profile request failed", true)
		}
		var body struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return User{}, err
		}
		if body.Email == "" {
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "google account email missing", false)
		}
		name := body.Name
		if name == "" {
			name = body.Email
		}
		return User{Email: body.Email, Name: name, Provider: ProviderGoogle, Subject: body.Sub}, nil
	case ProviderMicrosoft:
		resp, err := client.Get("https://graph.microsoft.com/v1.0/me")
		if err != nil {
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "failed to fetch microsoft profile", true)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "microsoft profile request failed: "+string(b), true)
		}
		var body struct {
			ID                string `json:"id"`
			DisplayName       string `json:"displayName"`
			Mail              string `json:"mail"`
			UserPrincipalName string `json:"userPrincipalName"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return User{}, err
		}
		email := body.Mail
		if email == "" {
			email = body.UserPrincipalName
		}
		if email == "" {
			return User{}, domain.NewAppError(domain.CodeUnauthorized, "microsoft account email missing", false)
		}
		name := body.DisplayName
		if name == "" {
			name = email
		}
		return User{Email: email, Name: name, Provider: ProviderMicrosoft, Subject: body.ID}, nil
	default:
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "unknown SSO provider", false)
	}
}

func (s *Service) issue(user User) (string, error) {
	p := tokenPayload{
		Email:    user.Email,
		Name:     user.Name,
		Provider: user.Provider,
		Subject:  user.Subject,
		Exp:      time.Now().Add(s.TokenTTL).Unix(),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func (s *Service) Parse(token string) (User, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "invalid token", false)
	}
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "invalid token", false)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "invalid token", false)
	}
	var p tokenPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "invalid token", false)
	}
	if time.Now().Unix() > p.Exp {
		return User{}, domain.NewAppError(domain.CodeUnauthorized, "token expired", false)
	}
	return User{Email: p.Email, Name: p.Name, Provider: p.Provider, Subject: p.Subject}, nil
}

// FrontendCallbackURL redirects the browser back to the SPA with the session token.
func (s *Service) FrontendCallbackURL(token string) string {
	u, err := url.Parse(s.WebOrigin + "/auth/callback")
	if err != nil {
		return s.WebOrigin + "/auth/callback?token=" + url.QueryEscape(token)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if user, err := s.Parse(strings.TrimPrefix(h, "Bearer ")); err == nil {
				ctx = context.WithValue(ctx, UserKey, user)
			}
		}
		r = r.WithContext(ctx)

		if s.Bypass || isPublic(r) || AllowsGuestAccess(r) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := UserFromContext(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		writeUnauthorized(w)
	})
}

func isPublic(r *http.Request) bool {
	path := r.URL.Path
	if path == "/healthz" || path == "/metrics" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}
	if r.Method == http.MethodOptions {
		return true
	}
	return false
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"authentication required","retryable":false}}`))
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(UserKey).(User)
	return u, ok
}
