package config

import (
	"context"
	"fmt"
	"regexp"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

// ShowCommand displays the current configuration
var ShowCommand = &cli.Command{
	Name:      "show",
	Usage:     "Display configuration with secrets redacted",
	ArgsUsage: "[config-file]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "show-secret-refs",
			Aliases: []string{"s"},
			Usage:   "show secret reference syntax (${secret:path:key}) instead of [SECRET]",
		},
	},
	Action: runShow,
}

func runShow(ctx context.Context, cmd *cli.Command) error {
	// Determine config path
	configPath := cmd.String("config")
	if cmd.Args().Len() > 0 {
		configPath = cmd.Args().First()
	}

	if configPath == "" {
		// Try to find default config
		path, err := config.FindConfig("stack")
		if err != nil {
			return fmt.Errorf("no config file specified and no default found: %w", err)
		}
		configPath = path
	}

	// Load the config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Redact secrets
	output := string(data)
	if cmd.Bool("show-secret-refs") {
		// Keep secret references visible
		// No modification needed - they're already in the YAML
	} else {
		// Replace secret references with [SECRET]
		secretPattern := regexp.MustCompile(`\$\{secret:[^}]+\}`)
		output = secretPattern.ReplaceAllString(output, "[SECRET]")
	}

	fmt.Printf("Configuration: %s\n", configPath)
	fmt.Println("---")
	fmt.Print(output)

	return nil
}
