package auth_test

import (
	"strings"
	"testing"

	"github.com/thanhtai9606/DocForge/apps/api/internal/auth"
)

func TestEnabledProvidersAndAuthURL(t *testing.T) {
	s := auth.New("test-secret", true, "http://localhost:5173", "http://localhost:8080",
		auth.ProviderConfig{ClientID: "g-client", ClientSecret: "g-secret"},
		auth.ProviderConfig{ClientID: "m-client", ClientSecret: "m-secret", Tenant: "common"},
	)
	providers := s.EnabledProviders()
	if len(providers) != 2 {
		t.Fatalf("providers=%v", providers)
	}
	gURL, err := s.AuthCodeURL(auth.ProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gURL, "accounts.google.com") {
		t.Fatalf("google url=%s", gURL)
	}
	mURL, err := s.AuthCodeURL(auth.ProviderMicrosoft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mURL, "login.microsoftonline.com") {
		t.Fatalf("microsoft url=%s", mURL)
	}
	cb := s.FrontendCallbackURL("abc.token")
	if !strings.Contains(cb, "/auth/callback") || !strings.Contains(cb, "token=") {
		t.Fatalf("callback=%s", cb)
	}
}

func TestMissingProviderConfig(t *testing.T) {
	s := auth.New("test-secret", true, "http://localhost:5173", "http://localhost:8080",
		auth.ProviderConfig{}, auth.ProviderConfig{})
	if len(s.EnabledProviders()) != 0 {
		t.Fatal("expected no providers")
	}
	if _, err := s.AuthCodeURL(auth.ProviderGoogle); err == nil {
		t.Fatal("expected error")
	}
}
