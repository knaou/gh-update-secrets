package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"gh-secret-sync/pkg/config"
)

type mockSyncer struct {
	syncRepoSecretFunc   func(ctx context.Context, owner, repo, secretName, secretValue string) error
	syncRepoVariableFunc func(ctx context.Context, owner, repo, varName, varValue string) error
	syncEnvSecretFunc    func(ctx context.Context, owner, repo, envName, secretName, secretValue string) error
	syncEnvVariableFunc  func(ctx context.Context, owner, repo, envName, varName, varValue string) error
}

func (m *mockSyncer) SyncRepositorySecret(ctx context.Context, owner, repo, secretName, secretValue string) error {
	if m.syncRepoSecretFunc != nil {
		return m.syncRepoSecretFunc(ctx, owner, repo, secretName, secretValue)
	}
	return nil
}

func (m *mockSyncer) SyncRepositoryVariable(ctx context.Context, owner, repo, varName, varValue string) error {
	if m.syncRepoVariableFunc != nil {
		return m.syncRepoVariableFunc(ctx, owner, repo, varName, varValue)
	}
	return nil
}

func (m *mockSyncer) SyncEnvironmentSecret(ctx context.Context, owner, repo, envName, secretName, secretValue string) error {
	if m.syncEnvSecretFunc != nil {
		return m.syncEnvSecretFunc(ctx, owner, repo, envName, secretName, secretValue)
	}
	return nil
}

func (m *mockSyncer) SyncEnvironmentVariable(ctx context.Context, owner, repo, envName, varName, varValue string) error {
	if m.syncEnvVariableFunc != nil {
		return m.syncEnvVariableFunc(ctx, owner, repo, envName, varName, varValue)
	}
	return nil
}

func TestRun(t *testing.T) {
	origLoadConfig := loadConfigFunc
	origNewClient := newClientFunc
	origEditSopsFile := editSopsFileFunc
	origInitSopsKey := initSopsKeyFunc
	defer func() {
		loadConfigFunc = origLoadConfig
		newClientFunc = origNewClient
		editSopsFileFunc = origEditSopsFile
		initSopsKeyFunc = origInitSopsKey
	}()

	t.Run("no subcommand error", func(t *testing.T) {
		err := run([]string{})
		if err == nil {
			t.Fatal("expected error when no subcommand provided, got nil")
		}
	})

	t.Run("help subcommands", func(t *testing.T) {
		for _, cmd := range []string{"help", "-h", "--help"} {
			if err := run([]string{cmd}); err != nil {
				t.Fatalf("unexpected error for %s: %v", cmd, err)
			}
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		err := run([]string{"unknown-cmd"})
		if err == nil {
			t.Fatal("expected error for unknown subcommand, got nil")
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		err := run([]string{"apply", "-invalidflag"})
		if err == nil {
			t.Fatal("expected error for invalid flag, got nil")
		}
	})

	t.Run("apply missing token", func(t *testing.T) {
		os.Unsetenv("GITHUB_TOKEN")
		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error due to missing token, got nil")
		}
	})

	t.Run("apply load config error", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return nil, errors.New("load failed")
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error due to load config failure, got nil")
		}
	})

	t.Run("apply empty repositories", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{Repos: []string{}}, nil
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err != nil {
			t.Fatalf("expected no error for empty repos, got %v", err)
		}
	})

	t.Run("apply success with invalid repo skip, repo secrets/vars and env secrets/vars", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{
				Repos: []string{"invalidrepo", "owner/repo1"},
				Secrets: map[string]string{
					"REPO_KEY": "REPO_VAL",
				},
				Vars: map[string]string{
					"REPO_VAR_KEY": "REPO_VAR_VAL",
				},
				Environments: []config.EnvironmentConfig{
					{
						Name: "production",
						Secrets: map[string]string{
							"ENV_KEY": "ENV_VAL",
						},
						Vars: map[string]string{
							"ENV_VAR_KEY": "ENV_VAR_VAL",
						},
					},
				},
			}, nil
		}

		repoSecretSynced := false
		repoVarSynced := false
		envSecretSynced := false
		envVarSynced := false

		newClientFunc = func(ctx context.Context, token string) resourceSyncer {
			return &mockSyncer{
				syncRepoSecretFunc: func(ctx context.Context, owner, repo, secretName, secretValue string) error {
					if owner == "owner" && repo == "repo1" && secretName == "REPO_KEY" && secretValue == "REPO_VAL" {
						repoSecretSynced = true
					}
					return nil
				},
				syncRepoVariableFunc: func(ctx context.Context, owner, repo, varName, varValue string) error {
					if owner == "owner" && repo == "repo1" && varName == "REPO_VAR_KEY" && varValue == "REPO_VAR_VAL" {
						repoVarSynced = true
					}
					return nil
				},
				syncEnvSecretFunc: func(ctx context.Context, owner, repo, envName, secretName, secretValue string) error {
					if owner == "owner" && repo == "repo1" && envName == "production" && secretName == "ENV_KEY" && secretValue == "ENV_VAL" {
						envSecretSynced = true
					}
					return nil
				},
				syncEnvVariableFunc: func(ctx context.Context, owner, repo, envName, varName, varValue string) error {
					if owner == "owner" && repo == "repo1" && envName == "production" && varName == "ENV_VAR_KEY" && varValue == "ENV_VAR_VAL" {
						envVarSynced = true
					}
					return nil
				},
			}
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !repoSecretSynced || !repoVarSynced || !envSecretSynced || !envVarSynced {
			t.Fatalf("expected resources to be synced, repoSecret=%v, repoVar=%v, envSecret=%v, envVar=%v", repoSecretSynced, repoVarSynced, envSecretSynced, envVarSynced)
		}
	})

	t.Run("apply repo secret sync error", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{
				Repos:   []string{"owner/repo1"},
				Secrets: map[string]string{"FAIL_KEY": "VAL"},
			}, nil
		}

		newClientFunc = func(ctx context.Context, token string) resourceSyncer {
			return &mockSyncer{
				syncRepoSecretFunc: func(ctx context.Context, owner, repo, secretName, secretValue string) error {
					return errors.New("sync repo secret error")
				},
			}
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error on repo secret sync failure, got nil")
		}
	})

	t.Run("apply repo variable sync error", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{
				Repos: []string{"owner/repo1"},
				Vars:  map[string]string{"FAIL_VAR_KEY": "VAL"},
			}, nil
		}

		newClientFunc = func(ctx context.Context, token string) resourceSyncer {
			return &mockSyncer{
				syncRepoVariableFunc: func(ctx context.Context, owner, repo, varName, varValue string) error {
					return errors.New("sync repo variable error")
				},
			}
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error on repo variable sync failure, got nil")
		}
	})

	t.Run("apply env secret sync error", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{
				Repos: []string{"owner/repo1"},
				Environments: []config.EnvironmentConfig{
					{
						Name:    "staging",
						Secrets: map[string]string{"FAIL_ENV_KEY": "VAL"},
					},
				},
			}, nil
		}

		newClientFunc = func(ctx context.Context, token string) resourceSyncer {
			return &mockSyncer{
				syncEnvSecretFunc: func(ctx context.Context, owner, repo, envName, secretName, secretValue string) error {
					return errors.New("sync env secret error")
				},
			}
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error on env secret sync failure, got nil")
		}
	})

	t.Run("apply env variable sync error", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "dummy-token")
		defer os.Unsetenv("GITHUB_TOKEN")

		loadConfigFunc = func(path string) (*config.Config, error) {
			return &config.Config{
				Repos: []string{"owner/repo1"},
				Environments: []config.EnvironmentConfig{
					{
						Name: "staging",
						Vars: map[string]string{"FAIL_ENV_VAR_KEY": "VAL"},
					},
				},
			}, nil
		}

		newClientFunc = func(ctx context.Context, token string) resourceSyncer {
			return &mockSyncer{
				syncEnvVariableFunc: func(ctx context.Context, owner, repo, envName, varName, varValue string) error {
					return errors.New("sync env variable error")
				},
			}
		}

		err := run([]string{"apply", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error on env variable sync failure, got nil")
		}
	})

	t.Run("edit subcommand success", func(t *testing.T) {
		editSopsFileFunc = func(path string) error {
			return nil
		}

		err := run([]string{"edit", "-c", "config.yaml"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("edit subcommand error", func(t *testing.T) {
		editSopsFileFunc = func(path string) error {
			return errors.New("edit failed")
		}

		err := run([]string{"edit", "-c", "config.yaml"})
		if err == nil {
			t.Fatal("expected error for edit subcommand, got nil")
		}
	})

	t.Run("init subcommand success", func(t *testing.T) {
		initSopsKeyFunc = func(keyPath, sopsConfigPath string) (*config.InitSopsResult, error) {
			return &config.InitSopsResult{
				KeyPath:        keyPath,
				PublicKey:      "age1dummy",
				SopsConfigPath: sopsConfigPath,
				CreatedSopsCfg: true,
			}, nil
		}

		err := run([]string{"init", "-o", "mykey.txt", "-sops-config", "mysops.yaml"})
		if err != nil {
			t.Fatalf("expected no error for init, got %v", err)
		}
	})

	t.Run("init subcommand error", func(t *testing.T) {
		initSopsKeyFunc = func(keyPath, sopsConfigPath string) (*config.InitSopsResult, error) {
			return nil, errors.New("init failed")
		}

		err := run([]string{"init"})
		if err == nil {
			t.Fatal("expected error for init subcommand, got nil")
		}
	})
}
