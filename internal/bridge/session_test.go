package bridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNULRequestRequiresToken(t *testing.T) {
	s := &Session{host: "dev", token: "secret"}
	called := false
	handler := s.handle(func(_ context.Context, request OpenRequest) (<-chan struct{}, error) {
		called = true
		if request.Host != "dev" || len(request.Argv) != 2 || request.Argv[1] != "with space" {
			t.Fatalf("request = %+v", request)
		}
		return nil, nil
	})
	body := strings.Join([]string{"wrong", "dev", "/tmp", ".", "with space"}, "\x00") + "\x00"
	request := httptest.NewRequest(http.MethodPost, "/v1/open", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-gitviewer-argv")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("bad token accepted: %d called=%v", response.Code, called)
	}

	body = strings.Join([]string{"secret", "dev", "/tmp", ".", "with space"}, "\x00") + "\x00"
	request = httptest.NewRequest(http.MethodPost, "/v1/open", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-gitviewer-argv")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK || !called {
		t.Fatalf("valid token rejected: %d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestNULRequestRequiresAbsoluteWorkingDirectory(t *testing.T) {
	s := &Session{host: "dev", token: "secret"}
	handler := s.handle(func(_ context.Context, _ OpenRequest) (<-chan struct{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	})
	body := strings.Join([]string{"secret", "dev", "relative", "."}, "\x00") + "\x00"
	request := httptest.NewRequest(http.MethodPost, "/v1/open", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-gitviewer-argv")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("relative cwd accepted: %d", response.Code)
	}
}

func TestHandlerFailureIsNotReportedAsOpened(t *testing.T) {
	s := &Session{host: "dev", token: "secret"}
	handler := s.handle(func(_ context.Context, _ OpenRequest) (<-chan struct{}, error) {
		return nil, errors.New("rejected")
	})
	body := strings.Join([]string{"secret", "dev", "/tmp", "."}, "\x00") + "\x00"
	request := httptest.NewRequest(http.MethodPost, "/v1/open", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-gitviewer-argv")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "rejected") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
