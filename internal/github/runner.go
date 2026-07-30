package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type ExecRunner struct {
	Path string
}

func NewExecRunner() (*ExecRunner, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("locate gh: %w", err)
	}
	return &ExecRunner{Path: path}, nil
}

func (r *ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	operation, err := validateReadOnlyCommand(args)
	if err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, r.Path, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return nil, &commandError{
			operation: operation,
			stderr:    strings.TrimSpace(stderr.String()),
			cause:     err,
		}
	}
	return stdout.Bytes(), nil
}

type commandError struct {
	operation string
	stderr    string
	cause     error
}

func (e *commandError) Error() string {
	if e.stderr == "" {
		return e.operation + " failed"
	}
	return e.operation + ": " + e.stderr
}

func (e *commandError) Unwrap() error {
	return e.cause
}

var mutationToken = regexp.MustCompile(`(?i)\bmutation\b`)

func validateReadOnlyCommand(args []string) (string, error) {
	if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
		if len(args) == 4 && args[2] == "--hostname" && args[3] == "github.com" {
			return "gh auth status --hostname github.com", nil
		}
		return "", errors.New("refusing GitHub authentication command: hostname must be exactly github.com")
	}
	if len(args) < 2 || args[0] != "api" || args[1] != "graphql" {
		return "", errors.New("refusing GitHub command: read-only runner permits only auth status and fixed GraphQL queries")
	}
	if len(args) < 6 || args[2] != "--hostname" || args[3] != "github.com" {
		return "", errors.New("refusing GitHub GraphQL command: hostname must be exactly github.com")
	}
	if (len(args)-4)%2 != 0 {
		return "", errors.New("refusing GitHub GraphQL command: flags must have values")
	}

	var document string
	var variables []string
	for index := 4; index < len(args); index += 2 {
		flag, value := args[index], args[index+1]
		switch flag {
		case "-f":
			name, rawValue, ok := strings.Cut(value, "=")
			if !ok || name == "" {
				return "", errors.New("refusing GitHub GraphQL command: raw fields must use name=value")
			}
			if name == "query" {
				if document != "" {
					return "", errors.New("refusing GitHub GraphQL command: exactly one query document is required")
				}
				document = rawValue
				continue
			}
			variables = append(variables, value)
		case "-F":
			return "", errors.New("refusing GitHub GraphQL command: typed fields are prohibited; variables must use raw -f fields")
		default:
			return "", fmt.Errorf("refusing GitHub GraphQL command: flag %q is not read-only approved", flag)
		}
	}

	if document == "" {
		return "", errors.New("refusing GitHub GraphQL command: exactly one query document is required")
	}
	if mutationToken.MatchString(document) {
		return "", errors.New("refusing GitHub GraphQL mutation: PRQ is read-only")
	}

	allowedVariables, ok := approvedQueryVariables(document)
	if !ok {
		return "", errors.New("refusing GitHub GraphQL command: document is not a fixed PRQ query")
	}
	if err := validateVariables(variables, allowedVariables); err != nil {
		return "", err
	}

	return "gh api graphql --hostname github.com", nil
}

func approvedQueryVariables(document string) (map[string]bool, bool) {
	switch document {
	case currentViewerQuery:
		return map[string]bool{}, true
	case viewerOrganizationsQuery:
		return map[string]bool{"cursor": false}, true
	case organizationRepositoriesQuery:
		return map[string]bool{"organization": true, "cursor": false}, true
	case openPullRequestsQuery:
		return map[string]bool{"search": true, "cursor": false}, true
	default:
		return nil, false
	}
}

func validateVariables(variables []string, allowed map[string]bool) error {
	seen := make(map[string]bool, len(variables))
	for _, variable := range variables {
		name, _, ok := strings.Cut(variable, "=")
		if _, declared := allowed[name]; !ok || name == "" || !declared {
			return fmt.Errorf("refusing GitHub GraphQL command: variable %q is not declared by the fixed query", name)
		}
		if seen[name] {
			return fmt.Errorf("refusing GitHub GraphQL command: variable %q is duplicated", name)
		}
		seen[name] = true
	}
	for name, required := range allowed {
		if required && !seen[name] {
			return fmt.Errorf("refusing GitHub GraphQL command: required variable %q is missing", name)
		}
	}
	return nil
}
