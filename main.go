package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"gh-secret-sync/pkg/config"
	"gh-secret-sync/pkg/github"
)

const requiredTokenScopeNotice = `
Note: Ensure your GITHUB_TOKEN has the following repository permissions:
  - Secrets: Read & Write
  - Variables: Read & Write
  - Environments: Read & Write
`

var (
	loadConfigFunc   = config.LoadSopsConfig
	editSopsFileFunc = config.EditSopsFile
	initSopsKeyFunc  = config.InitSopsKey
	newClientFunc    = func(ctx context.Context, token string) resourceSyncer {
		return github.NewClient(ctx, token)
	}
)

type resourceSyncer interface {
	SyncRepositorySecret(ctx context.Context, owner, repo, secretName, secretValue string) error
	SyncRepositoryVariable(ctx context.Context, owner, repo, varName, varValue string) error
	SyncEnvironmentSecret(ctx context.Context, owner, repo, envName, secretName, secretValue string) error
	SyncEnvironmentVariable(ctx context.Context, owner, repo, envName, varName, varValue string) error
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		printUsage()
		return fmt.Errorf("subcommand is required")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "init":
		return handleInit(subArgs)
	case "apply":
		fs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
		configFile := fs.String("c", "", "Path to SOPS config file")
		configFileLegacy := fs.String("f", "", "Path to SOPS config file (deprecated, use -c)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		path := *configFile
		if path == "" {
			path = *configFileLegacy
		}
		if path == "" {
			path = "config.sops.yaml"
		}
		return handleApply(path, fs.Args())
	case "edit":
		fs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
		configFile := fs.String("c", "", "Path to SOPS config file")
		configFileLegacy := fs.String("f", "", "Path to SOPS config file (deprecated, use -c)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		path := *configFile
		if path == "" {
			path = *configFileLegacy
		}
		if path == "" {
			path = "config.sops.yaml"
		}
		return handleEdit(path)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand %q", subcommand)
	}
}

func handleInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	keyPath := fs.String("o", "key.txt", "Path to output Age key file")
	sopsConfigPath := fs.String("sops-config", ".sops.yaml", "Path to .sops.yaml file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	res, err := initSopsKeyFunc(*keyPath, *sopsConfigPath)
	if err != nil {
		return fmt.Errorf("failed to initialize sops key: %w", err)
	}

	fmt.Printf("Successfully generated SOPS Age key!\n")
	fmt.Printf("  Public key:  %s\n", res.PublicKey)
	fmt.Printf("  Key file:    %s (chmod 0600)\n", res.KeyPath)
	if res.CreatedSopsCfg {
		fmt.Printf("  SOPS config: %s\n", res.SopsConfigPath)
	} else {
		fmt.Printf("  SOPS config: %s (already exists, skipping creation)\n", res.SopsConfigPath)
	}
	if res.KeyPath == config.DefaultAgeKeyFile {
		fmt.Printf("\ngh-secret-sync automatically uses %s when SOPS_AGE_KEY and SOPS_AGE_KEY_FILE are not set.\n", config.DefaultAgeKeyFile)
	} else {
		fmt.Println("\nTo use this key with gh-secret-sync and SOPS, set:")
		fmt.Printf("  export SOPS_AGE_KEY_FILE=%s\n", res.KeyPath)
	}
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run 'gh-secret-sync edit' to create and edit your encrypted config file.")
	fmt.Println("  2. Run 'gh-secret-sync apply' to sync secrets and variables to GitHub repositories.")

	return nil
}

func handleApply(configPath string, repoFilters []string) error {
	cfg, err := loadConfigFunc(configPath)
	if err != nil {
		return fmt.Errorf("failed to load sops config: %w", err)
	}

	// Build a set of repo filters for O(1) lookup
	filterSet := make(map[string]struct{}, len(repoFilters))
	for _, r := range repoFilters {
		filterSet[r] = struct{}{}
	}

	// Resolve token: config settings.github_token takes precedence over env var
	token := cfg.Settings.GitHubToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN is not set. Specify settings.github_token in config or set GITHUB_TOKEN environment variable.%s", requiredTokenScopeNotice)
	}

	if len(cfg.Repos) == 0 {
		fmt.Println("No repositories specified in config.")
		return nil
	}

	ctx := context.Background()
	ghClient := newClientFunc(ctx, token)

	fmt.Printf("Starting resource synchronization from %s...\n", configPath)
	fmt.Print(requiredTokenScopeNotice)

	for _, repoEntry := range cfg.Repos {
		parts := strings.SplitN(repoEntry.Repo, "/", 2)
		if len(parts) != 2 {
			fmt.Printf("[SKIP] Invalid repository format %q. Expected 'owner/repo'.\n", repoEntry.Repo)
			continue
		}

		// If repo filters are specified, skip repos not in the filter set
		if len(filterSet) > 0 {
			if _, ok := filterSet[repoEntry.Repo]; !ok {
				continue
			}
		}
		owner, repo := parts[0], parts[1]

		fmt.Printf("Processing repository: %s/%s\n", owner, repo)

		// Sync repository secrets
		for name, val := range cfg.Secrets {
			if !repoEntry.ShouldInclude(name) {
				continue
			}
			fmt.Printf("  -> Syncing repo secret: %s\n", name)
			if err := ghClient.SyncRepositorySecret(ctx, owner, repo, name, val); err != nil {
				return fmt.Errorf("failed to sync repo secret %s for %s/%s: %w", name, owner, repo, err)
			}
		}

		// Sync repository variables
		for name, val := range cfg.Vars {
			if !repoEntry.ShouldInclude(name) {
				continue
			}
			fmt.Printf("  -> Syncing repo variable: %s\n", name)
			if err := ghClient.SyncRepositoryVariable(ctx, owner, repo, name, val); err != nil {
				return fmt.Errorf("failed to sync repo variable %s for %s/%s: %w", name, owner, repo, err)
			}
		}

		// Sync environment secrets and variables
		for _, envCfg := range cfg.Environments {
			if !repoEntry.ShouldInclude(envCfg.Name) {
				continue
			}
			fmt.Printf("  -> Processing environment: %s\n", envCfg.Name)
			for name, val := range envCfg.Secrets {
				fmt.Printf("     -> Syncing env secret: %s\n", name)
				if err := ghClient.SyncEnvironmentSecret(ctx, owner, repo, envCfg.Name, name, val); err != nil {
					return fmt.Errorf("failed to sync env secret %s for %s/%s (env %s): %w", name, owner, repo, envCfg.Name, err)
				}
			}
			for name, val := range envCfg.Vars {
				fmt.Printf("     -> Syncing env variable: %s\n", name)
				if err := ghClient.SyncEnvironmentVariable(ctx, owner, repo, envCfg.Name, name, val); err != nil {
					return fmt.Errorf("failed to sync env variable %s for %s/%s (env %s): %w", name, owner, repo, envCfg.Name, err)
				}
			}
		}
	}

	fmt.Println("Successfully synchronized all secrets and variables.")
	return nil
}

func handleEdit(configPath string) error {
	fmt.Printf("Opening %s for safe editing...\n", configPath)
	if err := editSopsFileFunc(configPath); err != nil {
		return err
	}
	fmt.Printf("Successfully saved and re-encrypted %s\n", configPath)
	return nil
}

func printUsage() {
	usage := `Usage: gh-secret-sync <subcommand> [flags] [args...]

Subcommands:
  init     Generate a new SOPS Age key and .sops.yaml file
  apply    Decrypt config and sync secrets and variables to GitHub repositories
  edit     Decrypt, edit, and re-encrypt config file safely

Flags for init:
  -o <path>           Path to output Age key file (default: key.txt)
  -sops-config <path> Path to .sops.yaml file (default: .sops.yaml)

Flags for apply:
  -c <path>   Path to SOPS config file (default: config.sops.yaml)
  [repos...]  Optional list of repositories (owner/repo) to sync.
              If omitted, all repositories in the config are synced.
              Example: gh-secret-sync apply owner/repo1 owner/repo2

Flags for edit:
  -c <path>   Path to SOPS config file (default: config.sops.yaml)

Config Settings (in SOPS config file):
  settings.github_token  GitHub Personal Access Token (takes precedence over GITHUB_TOKEN env var)

Environment Variables:
  GITHUB_TOKEN       GitHub Personal Access Token (fallback when settings.github_token is not set)
                     Required scopes: Secrets (Read & Write), Variables (Read & Write), Environments (Read & Write)
  SOPS_AGE_KEY       Age secret key used by SOPS to decrypt the configuration file (takes precedence over SOPS_AGE_KEY_FILE)
  SOPS_AGE_KEY_FILE  Path to the Age secret key file used by SOPS to decrypt the configuration file
                     If neither is set, key.txt in the current directory is used by default.
`
	fmt.Print(usage)
}
