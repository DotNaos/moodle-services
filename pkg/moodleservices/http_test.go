package moodleservices

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestParsePositiveIntEnvUsesConfiguredValue(t *testing.T) {
	t.Setenv("MOODLE_TEST_QUOTA", "12345")

	if got := parsePositiveIntEnv("MOODLE_TEST_QUOTA", 99); got != 12345 {
		t.Fatalf("quota = %d, want 12345", got)
	}
}

func TestParsePositiveIntEnvFallsBackForInvalidValue(t *testing.T) {
	tests := []string{"", "0", "-1", "nope"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MOODLE_TEST_QUOTA", value)
			if got := parsePositiveIntEnv("MOODLE_TEST_QUOTA", 99); got != 99 {
				t.Fatalf("quota = %d, want fallback 99", got)
			}
		})
	}
}

func TestParsePositiveInt64EnvUsesConfiguredValue(t *testing.T) {
	t.Setenv("MOODLE_TEST_QUOTA_64", "5368709120")

	if got := parsePositiveInt64Env("MOODLE_TEST_QUOTA_64", 99); got != 5368709120 {
		t.Fatalf("quota = %d, want 5368709120", got)
	}
}

func TestLoadServerEnvUsesDefaultCodexQuotas(t *testing.T) {
	t.Setenv(EnvCodexStateQuota, "")
	t.Setenv(EnvCodexStateAdminQuota, "")

	cfg := LoadServerEnv()

	if cfg.CodexStateUserQuotaBytes != 512*1024*1024 {
		t.Fatalf("user quota = %d, want 512 MiB", cfg.CodexStateUserQuotaBytes)
	}
	if cfg.CodexStateAdminQuotaBytes != 1024*1024*1024 {
		t.Fatalf("admin quota = %d, want 1 GiB", cfg.CodexStateAdminQuotaBytes)
	}
}

func TestLoadServerEnvParsesConfiguredAdminClerkUsers(t *testing.T) {
	t.Setenv(EnvAdminClerkUsers, " user_1, user_2 ,, ")

	cfg := LoadServerEnv()

	if !cfg.IsConfiguredAdminClerkUser("user_1") || !cfg.IsConfiguredAdminClerkUser("user_2") {
		t.Fatalf("expected configured admin users to be recognized")
	}
	if cfg.IsConfiguredAdminClerkUser("user_3") {
		t.Fatalf("unexpected admin user")
	}
}

func TestServiceForRequestRejectsGlobalSessionFallbackWithoutDatabase(t *testing.T) {
	t.Setenv("MOODLE_MOBILE_SESSION_JSON", `{"siteUrl":"https://moodle.fhgr.ch","userId":123,"token":"global-token"}`)

	request := httptest.NewRequest("GET", "/api/courses", nil)
	request.Header.Set("X-Moodle-App-Key", "moodle_test_key")

	service, closeFn, err := ServiceForRequest(request, ServerEnv{})
	if err == nil {
		if closeFn != nil {
			closeFn()
		}
		t.Fatalf("ServiceForRequest unexpectedly returned service: %#v", service)
	}
	if !errors.Is(err, ErrDatabaseNotConfigured) {
		t.Fatalf("ServiceForRequest error = %v, want ErrDatabaseNotConfigured", err)
	}
	if closeFn != nil {
		t.Fatalf("ServiceForRequest returned closeFn despite failing before opening a store")
	}
}
