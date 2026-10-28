package askpass

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrokerPromptsAndRejectsInvalidToken(t *testing.T) {
	broker, err := Start(func(_ context.Context, request Request) (string, error) {
		if request.Host != "dev-box" || request.Prompt != "Password:" {
			t.Fatalf("request = %+v", request)
		}
		return "correct horse battery staple", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	if err := broker.InstallEnvironment(); err != nil {
		t.Fatal(err)
	}
	socket := os.Getenv("GITVIEWER_ASKPASS_SOCK")
	token := os.Getenv("GITVIEWER_ASKPASS_TOKEN")
	if socket == "" || token == "" || os.Getenv("SSH_ASKPASS_REQUIRE") != "force" {
		t.Fatal("askpass environment was not installed")
	}
	info, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v", info.Mode().Perm())
	}

	response := askpassRoundTrip(t, socket, wireRequest{Token: token, Host: "dev-box", Prompt: "Password:"})
	if response.Error != "" || response.Password != "correct horse battery staple" {
		t.Fatalf("response = %+v", response)
	}
	response = askpassRoundTrip(t, socket, wireRequest{Token: "wrong", Host: "dev-box", Prompt: "Password:"})
	if response.Error == "" || response.Password != "" {
		t.Fatalf("invalid token response = %+v", response)
	}
}

func TestBrokerCachesPasswordPerHostAndClearsOnFailure(t *testing.T) {
	var prompts atomic.Int32
	broker, err := Start(func(context.Context, Request) (string, error) {
		prompts.Add(1)
		return "cached-secret", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	if err := broker.InstallEnvironment(); err != nil {
		t.Fatal(err)
	}
	socket := os.Getenv("GITVIEWER_ASKPASS_SOCK")
	token := os.Getenv("GITVIEWER_ASKPASS_TOKEN")
	request := wireRequest{Token: token, Host: "dev-box", Prompt: "dev-box password:"}
	for i := 0; i < 2; i++ {
		response := askpassRoundTrip(t, socket, request)
		if response.Password != "cached-secret" {
			t.Fatalf("response = %+v", response)
		}
	}
	if prompts.Load() != 1 {
		t.Fatalf("prompts = %d", prompts.Load())
	}
	broker.clearHost("dev-box")
	response := askpassRoundTrip(t, socket, request)
	if response.Password != "cached-secret" || prompts.Load() != 2 {
		t.Fatalf("response = %+v prompts = %d", response, prompts.Load())
	}
}

func TestBrokerRechecksCacheAfterConcurrentPromptWait(t *testing.T) {
	var prompts atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	broker, err := Start(func(context.Context, Request) (string, error) {
		if prompts.Add(1) == 1 {
			close(entered)
			<-release
		}
		return "cached-secret", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	if err := broker.InstallEnvironment(); err != nil {
		t.Fatal(err)
	}
	socket := os.Getenv("GITVIEWER_ASKPASS_SOCK")
	token := os.Getenv("GITVIEWER_ASKPASS_TOKEN")
	request := wireRequest{Token: token, Host: "dev-box", Prompt: "dev-box password:"}

	type result struct {
		response wireResponse
		err      error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		response, err := askpassRoundTripResult(socket, request)
		results <- result{response: response, err: err}
	}()
	<-entered
	go func() {
		defer workers.Done()
		response, err := askpassRoundTripResult(socket, request)
		results <- result{response: response, err: err}
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	workers.Wait()
	close(results)
	for result := range results {
		if result.err != nil || result.response.Error != "" || result.response.Password != "cached-secret" {
			t.Fatalf("result = %+v", result)
		}
	}
	if prompts.Load() != 1 {
		t.Fatalf("prompts = %d", prompts.Load())
	}
}

func askpassRoundTrip(t *testing.T, socket string, request wireRequest) wireResponse {
	t.Helper()
	response, err := askpassRoundTripResult(socket, request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func askpassRoundTripResult(socket string, request wireRequest) (wireResponse, error) {
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return wireResponse{}, err
	}
	defer conn.Close()
	data, err := json.Marshal(request)
	if err != nil {
		return wireResponse{}, err
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return wireResponse{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return wireResponse{}, err
	}
	var response wireResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return wireResponse{}, err
	}
	return response, nil
}

func TestRunHelperStripsFlagFromPrompt(t *testing.T) {
	if !IsHelper([]string{"git-viewer", HelperFlag}) {
		t.Fatal("helper flag not detected")
	}
	if got := helperPrompt([]string{HelperFlag, "Password:"}); got != "Password:" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestRunHelperRoundTrip(t *testing.T) {
	broker, err := Start(func(_ context.Context, request Request) (string, error) {
		if request.Host != "dev-box" || request.Prompt != "Password:" {
			t.Errorf("request = %+v", request)
		}
		return "helper-secret", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	if err := broker.InstallEnvironment(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITVIEWER_ASKPASS_HOST", "dev-box")

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	code := RunHelper([]string{HelperFlag, "Password:"})
	os.Stdout = oldStdout
	_ = writer.Close()
	output, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || string(output) != "helper-secret\n" {
		t.Fatalf("code = %d output = %q", code, output)
	}
}
