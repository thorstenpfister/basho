package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/service"
)

// configCommand is `basho config path|show` (SPEC §12).
func (a *app) configCommand() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Where the configuration lives and what is in effect", Args: usageArgs(cobra.NoArgs)}
	c.AddCommand(&cobra.Command{
		Use: "path", Short: "Print the config file's path (it may not exist yet)", Args: usageArgs(cobra.NoArgs),
		// Without loading the file, so a broken file can still be found.
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := a.configFile()
			if err != nil {
				return err
			}
			exists := fileExists(path)
			if a.json {
				return writeJSON(cmd.OutOrStdout(), "config_path", service.Provenance{Source: "basho"}, map[string]any{"path": path, "exists": exists})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			if !exists {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No file there yet: basho runs on its defaults. basho config show prints them.")
			}
			return err
		},
	}, &cobra.Command{
		Use: "show", Short: "Print the configuration in effect: defaults, file, BASHO_* variables and flags", Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.setup(cmd); err != nil {
				return err
			}
			text, err := a.cfg.Encode()
			if err != nil {
				return err
			}
			if a.json {
				var m map[string]any
				if _, err := toml.Decode(text, &m); err != nil {
					return err
				}
				return writeJSON(cmd.OutOrStdout(), "config", service.Provenance{Source: "basho"}, m)
			}
			head, err := a.configHead()
			if err != nil {
				return err
			}
			defaults, err := config.Default().Encode()
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), head+"\n"+markChanged(text, defaults))
			return err
		},
	})
	return c
}

// configHead is the comment above `config show`: where everything lives and
// the session switches, which the file never holds.
func (a *app) configHead() (string, error) {
	path, err := a.configFile()
	if err != nil {
		return "", err
	}
	if !fileExists(path) {
		path += " (not there: defaults)"
	}
	cache, err := a.cfg.CacheDir(a.deps.Env)
	if err != nil {
		return "", err
	}
	state, err := config.StateDir(a.deps.Env)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# The configuration in effect: defaults, the file, BASHO_* variables, flags.\n"+
		"# file   %s\n# cache  %s\n# state  %s\n# this run: source = %s, offline = %t, debug = %t\n",
		path, cache, state, a.cfg.Source, a.cfg.Offline, a.cfg.Debug), nil
}

// markChanged notes the default after every setting that differs from it.
// Both texts come from Encode, so their lines pair up.
func markChanged(text, defaults string) string {
	lines, def := strings.Split(text, "\n"), strings.Split(defaults, "\n")
	for i, l := range lines {
		if i < len(def) && l != def[i] {
			if _, v, ok := strings.Cut(def[i], " = "); ok {
				lines[i] += "  # default: " + v
			}
		}
	}
	return strings.Join(lines, "\n")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}
