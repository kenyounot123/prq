package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

var ErrUnsupportedPlatform = errors.New("unsupported platform")

type Executor interface {
	Run(ctx context.Context, name string, args []string, stdin string) error
}

type SystemExecutor struct{}

func (SystemExecutor) Run(ctx context.Context, name string, args []string, stdin string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("run %s: %w: %s", name, err, detail)
		}
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

type Clipboard struct {
	Exec Executor
	GOOS string
}

type Browser struct {
	Exec Executor
	GOOS string
}

func (c Clipboard) Copy(ctx context.Context, text string) error {
	if c.GOOS != "darwin" {
		return fmt.Errorf("copy to clipboard: %w: %s", ErrUnsupportedPlatform, c.GOOS)
	}
	if err := c.Exec.Run(ctx, "pbcopy", []string{}, text); err != nil {
		return fmt.Errorf("copy to clipboard: %w", err)
	}
	return nil
}

func (b Browser) Open(ctx context.Context, rawURL string) error {
	if b.GOOS != "darwin" {
		return fmt.Errorf("open browser: %w: %s", ErrUnsupportedPlatform, b.GOOS)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "github.com" || parsed.User != nil || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/") {
		return fmt.Errorf("open browser: URL must be an HTTPS github.com URL without credentials or fragment")
	}
	if err := b.Exec.Run(ctx, "open", []string{rawURL}, ""); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
