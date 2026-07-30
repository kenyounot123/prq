package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin string
}

type fakeExecutor struct {
	calls []call
	err   error
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string, stdin string) error {
	f.calls = append(f.calls, call{name: name, args: slices.Clone(args), stdin: stdin})
	return f.err
}

func TestClipboardCopyInvokesPBcopyWithExactText(t *testing.T) {
	executor := &fakeExecutor{}
	clipboard := Clipboard{Exec: executor, GOOS: "darwin"}

	if err := clipboard.Copy(context.Background(), "• copied PR URL"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	want := []call{{name: "pbcopy", args: []string{}, stdin: "• copied PR URL"}}
	if !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("calls = %#v, want %#v", executor.calls, want)
	}
}

func TestBrowserOpenInvokesOpenWithURLAsOneArgument(t *testing.T) {
	executor := &fakeExecutor{}
	browser := Browser{Exec: executor, GOOS: "darwin"}
	const rawURL = "https://github.com/acme/api/pull/184"

	if err := browser.Open(context.Background(), rawURL); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	want := []call{{name: "open", args: []string{rawURL}, stdin: ""}}
	if !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("calls = %#v, want %#v", executor.calls, want)
	}
}

func TestBrowserOpenRejectsUnsafeURLsBeforeLaunching(t *testing.T) {
	tests := map[string]string{
		"HTTP":                "http://github.com/acme/api/pull/184",
		"malformed":           "https://github.com/%zz",
		"other host":          "https://example.com/acme/api/pull/184",
		"credentials":         "https://user:password@github.com/acme/api/pull/184",
		"fragment":            "https://github.com/acme/api/pull/184#comments",
		"missing path":        "https://github.com",
		"relative path":       "/acme/api/pull/184",
		"subdomain lookalike": "https://github.com.example/acme/api/pull/184",
	}

	for name, rawURL := range tests {
		t.Run(name, func(t *testing.T) {
			executor := &fakeExecutor{}
			browser := Browser{Exec: executor, GOOS: "darwin"}

			if err := browser.Open(context.Background(), rawURL); err == nil {
				t.Fatalf("Open(%q) error = nil, want invalid URL error", rawURL)
			}
			if len(executor.calls) != 0 {
				t.Fatalf("Open(%q) launched %#v, want no command", rawURL, executor.calls)
			}
		})
	}
}

func TestActionsPreserveOperationContextForExecutorFailures(t *testing.T) {
	executorErr := errors.New("command failed")
	tests := map[string]struct {
		run  func(context.Context, *fakeExecutor) error
		want string
	}{
		"clipboard": {
			run: func(ctx context.Context, executor *fakeExecutor) error {
				return Clipboard{Exec: executor, GOOS: "darwin"}.Copy(ctx, "copied text")
			},
			want: "copy to clipboard",
		},
		"browser": {
			run: func(ctx context.Context, executor *fakeExecutor) error {
				return Browser{Exec: executor, GOOS: "darwin"}.Open(ctx, "https://github.com/acme/api/pull/184")
			},
			want: "open browser",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.run(context.Background(), &fakeExecutor{err: executorErr})
			if !errors.Is(err, executorErr) {
				t.Fatalf("error = %v, want wrapped %v", err, executorErr)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want operation context %q", err, tt.want)
			}
		})
	}
}

func TestActionsRejectUnsupportedPlatformsWithoutLaunching(t *testing.T) {
	tests := map[string]struct {
		run func(context.Context, *fakeExecutor) error
	}{
		"clipboard": {
			run: func(ctx context.Context, executor *fakeExecutor) error {
				return Clipboard{Exec: executor, GOOS: "linux"}.Copy(ctx, "copied text")
			},
		},
		"browser": {
			run: func(ctx context.Context, executor *fakeExecutor) error {
				return Browser{Exec: executor, GOOS: "linux"}.Open(ctx, "https://github.com/acme/api/pull/184")
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			executor := &fakeExecutor{}
			err := tt.run(context.Background(), executor)
			if !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("error = %v, want unsupported-platform error", err)
			}
			if len(executor.calls) != 0 {
				t.Fatalf("calls = %#v, want no command", executor.calls)
			}
		})
	}
}

func TestSystemExecutorRunPassesStdinToCommand(t *testing.T) {
	t.Setenv("PRQ_PLATFORM_HELPER", "success")

	err := (SystemExecutor{}).Run(
		context.Background(),
		os.Args[0],
		[]string{"-test.run=^TestSystemExecutorHelperProcess$"},
		"exact clipboard text\n",
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestSystemExecutorRunReportsStderrWithoutStdout(t *testing.T) {
	t.Setenv("PRQ_PLATFORM_HELPER", "failure")

	err := (SystemExecutor{}).Run(
		context.Background(),
		os.Args[0],
		[]string{"-test.run=^TestSystemExecutorHelperProcess$"},
		"",
	)
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}
	if !strings.Contains(err.Error(), "concise stderr") {
		t.Fatalf("Run() error = %q, want stderr context", err)
	}
	if strings.Contains(err.Error(), "private stdout") {
		t.Fatalf("Run() error exposed stdout: %q", err)
	}
}

func TestSystemExecutorHelperProcess(t *testing.T) {
	switch os.Getenv("PRQ_PLATFORM_HELPER") {
	case "success":
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil || string(stdin) != "exact clipboard text\n" {
			fmt.Fprintln(os.Stderr, "stdin mismatch")
			os.Exit(2)
		}
	case "failure":
		fmt.Fprint(os.Stdout, "private stdout")
		fmt.Fprint(os.Stderr, "concise stderr")
		os.Exit(7)
	}
}
