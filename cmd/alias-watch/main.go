package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lorenzbischof/alias-watch/internal/aliasescsv"
	"github.com/lorenzbischof/alias-watch/internal/config"
	"github.com/lorenzbischof/alias-watch/internal/db"
	imaputil "github.com/lorenzbischof/alias-watch/internal/imap"
	"github.com/lorenzbischof/alias-watch/internal/notify"
	"github.com/lorenzbischof/alias-watch/internal/report"
	"github.com/lorenzbischof/alias-watch/internal/tui"
)

var cfgPath string

func main() {
	root := &cobra.Command{
		Use:   "alias-watch",
		Short: "Monitor email aliases for unexpected senders",
	}
	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")

	root.AddCommand(
		cmdImport(),
		cmdValidate(),
		cmdLearn(),
		cmdMonitor(),
		cmdReport(),
		cmdFlag(),
		cmdTUI(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, error) {
	return config.Load(cfgPath)
}

func openStore(cfg *config.Config) (*db.Store, error) {
	return db.Open(cfg.DB.Path)
}

func requireIMAPConfig(cfg *config.Config) error {
	if strings.TrimSpace(cfg.IMAP.Server) == "" {
		return fmt.Errorf("missing IMAP server: set imap.server in config or IMAP_SERVER")
	}
	if strings.TrimSpace(cfg.IMAP.Username) == "" {
		return fmt.Errorf("missing IMAP username: set imap.username in config or IMAP_USERNAME")
	}
	if strings.TrimSpace(cfg.IMAP.Password()) == "" {
		return fmt.Errorf("missing IMAP password: set IMAP_PASSWORD")
	}
	return nil
}

// --- import ---

func cmdImport() *cobra.Command {
	return &cobra.Command{
		Use:   "import <path|->",
		Short: "Import aliases from an addy.io CSV export",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			var in = cmd.InOrStdin()
			if args[0] != "-" {
				f, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("open csv: %w", err)
				}
				defer f.Close()
				in = f
			}

			aliases, err := aliasescsv.Parse(in)
			if err != nil {
				return fmt.Errorf("parse csv: %w", err)
			}

			now := time.Now()
			for _, a := range aliases {
				if err := store.UpsertAlias(db.Alias{
					Email:    a.Email,
					AddyID:   a.ID,
					Active:   a.Active,
					Title:    a.Description,
					SyncedAt: now,
				}); err != nil {
					return fmt.Errorf("upsert alias %s: %w", a.Email, err)
				}
			}

			fmt.Printf("Imported %d aliases from %s\n", len(aliases), args[0])
			return nil
		},
	}
}

// --- validate ---

func cmdValidate() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Cross-validate data sources, print issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			aliases, err := store.AllAliases()
			if err != nil {
				return err
			}

			issues := 0
			for _, alias := range aliases {
				accounts, err := store.AccountsForAlias(alias.Email)
				if err != nil {
					return err
				}
				if len(accounts) == 0 {
					fmt.Printf("[UNMAPPED]    %s\n", alias.Email)
					issues++
				}

				emailCount, _ := store.EmailCountForAlias(alias.Email)
				if len(accounts) > 0 && emailCount == 0 {
					fmt.Printf("[NO_EMAILS]   %s\n", alias.Email)
					issues++
				}

				senderCount, _ := store.KnownSenderCountForAlias(alias.Email)
				if len(accounts) > 0 && senderCount == 0 {
					fmt.Printf("[NO_HISTORY]  %s\n", alias.Email)
					issues++
				}

				if !alias.Active && len(accounts) > 0 {
					fmt.Printf("[DELETED]     %s\n", alias.Email)
					issues++
				}
			}

			if issues == 0 {
				fmt.Println("No issues found.")
				return nil
			}
			return fmt.Errorf("%d issue(s) found", issues)
		},
	}
}

// --- learn ---

func cmdLearn() *cobra.Command {
	var debug bool
	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Scan IMAP history to populate known_senders",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if err := requireIMAPConfig(cfg); err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			imapClient, err := imaputil.Connect(cfg.IMAP)
			if err != nil {
				return fmt.Errorf("imap connect: %w", err)
			}
			defer imapClient.Close()

			count, err := imaputil.Learn(imaputil.LearnOptions{
				Client: imapClient,
				Store:  store,
				Debug:  debug,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Learned %d sender records\n", count)
			return nil
		},
	}
	cmd.Flags().BoolVar(&debug, "debug", false, "print debug info for each message processed")
	return cmd
}

// --- monitor ---

func cmdMonitor() *cobra.Command {
	return &cobra.Command{
		Use:   "monitor",
		Short: "IMAP IDLE daemon — alert on new/flagged senders",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if err := requireIMAPConfig(cfg); err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			notifier := notify.NewClient(cfg.Notify.NtfyURL, cfg.Notify.NtfyToken)

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			fmt.Println("Starting IDLE monitor (Ctrl+C to stop)...")
			return imaputil.Monitor(ctx, imaputil.MonitorOptions{
				IMAPConfig: cfg.IMAP,
				Store:      store,
				Notifier:   notifier,
			})
		},
	}
}

// --- report ---

func cmdReport() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Print alias→account table to stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			aliases, err := store.AllAliases()
			if err != nil {
				return err
			}

			knownSenders, err := store.AllKnownSenders()
			if err != nil {
				return err
			}
			senderMap := make(map[string][]string)
			for _, ks := range knownSenders {
				senderMap[ks.AliasEmail] = append(senderMap[ks.AliasEmail], ks.SenderEmail)
			}

			report.Print(os.Stdout, aliases, senderMap)
			return nil
		},
	}
}

// --- tui ---

func cmdTUI() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive two-pane TUI for managing known senders",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()
			m, err := tui.New(store)
			if err != nil {
				return err
			}
			_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
			return err
		},
	}
}

// --- flag ---

func cmdFlag() *cobra.Command {
	return &cobra.Command{
		Use:   "flag <email-id>",
		Short: "Flag email + sender as phishing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := openStore(cfg)
			if err != nil {
				return err
			}
			defer store.Close()

			if err := store.FlagEmail(id); err != nil {
				return err
			}
			fmt.Printf("Flagged email %d and its sender as phishing.\n", id)
			return nil
		},
	}
}
