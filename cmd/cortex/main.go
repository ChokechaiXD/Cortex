package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cortex.local/cortex/internal/config"
	"cortex.local/cortex/internal/cortex"
	"cortex.local/cortex/internal/hermes"
	"cortex.local/cortex/internal/httpapi"
	holographicimport "cortex.local/cortex/internal/importer/holographic"
)

const version = "0.1.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "agent":
		return runAgent(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "connector":
		return runConnector(args[1:], stdout, stderr)
	case "import":
		return runImport(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runImport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "holographic" {
		fmt.Fprintln(stderr, "usage: cortex import holographic --database MEMORY_STORE_DB --agent AGENT")
		return 2
	}
	flags := flag.NewFlagSet("import holographic", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", config.DefaultDataDir(), "Cortex data directory")
	databasePath := flags.String("database", "", "Holographic memory_store.db path")
	agentID := flags.String("agent", "", "agent that owned the legacy database")
	project := flags.String("project", "", "project scope for legacy project facts")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	file, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	hub, err := cortex.Open(cortex.Config{
		DatabasePath: config.DatabasePath(*dataDir),
		AdminAgents:  file.AdminAgents,
	})
	if err != nil {
		fmt.Fprintf(stderr, "open Cortex: %v\n", err)
		return 1
	}
	defer hub.Close()
	result, err := holographicimport.Import(context.Background(), hub, holographicimport.Options{
		DatabasePath: *databasePath,
		AgentID:      *agentID,
		Project:      *project,
	})
	if err != nil {
		fmt.Fprintf(stderr, "import Holographic memory: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "imported=%d\nreplayed=%d\n", result.Imported, result.Replayed)
	return 0
}

func runConnector(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "sync" || args[1] != "hermes" {
		fmt.Fprintln(stderr, "usage: cortex connector sync hermes --home HERMES_HOME")
		return 2
	}
	flags := flag.NewFlagSet("connector sync hermes", flag.ContinueOnError)
	flags.SetOutput(stderr)
	hermesHome := flags.String("home", "", "root Hermes home")
	dataDir := flags.String("data-dir", config.DefaultDataDir(), "Cortex data directory")
	serverURL := flags.String("url", "http://127.0.0.1:7777", "Cortex server URL")
	rootAgent := flags.String("root-agent", "mika", "agent id for the root Hermes profile")
	activate := flags.Bool("activate", true, "set memory.provider to cortex in every profile")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	result, err := hermes.Sync(hermes.SyncOptions{
		HermesHome: *hermesHome,
		DataDir:    *dataDir,
		ServerURL:  *serverURL,
		RootAgent:  *rootAgent,
		Activate:   *activate,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sync Hermes connector: %v\n", err)
		return 1
	}
	for _, profile := range result.Profiles {
		fmt.Fprintf(stdout, "profile=%s\nhome=%s\n", profile.AgentID, profile.Home)
	}
	return 0
}

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", config.DefaultDataDir(), "Cortex data directory")
	admin := flags.String("admin", "mika", "initial administrator agent id")
	listen := flags.String("listen", "127.0.0.1:7777", "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	_, token, err := config.Initialize(*dataDir, *admin, *listen)
	if err != nil {
		fmt.Fprintf(stderr, "initialize Cortex: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "data_dir=%s\nagent=%s\ntoken=%s\n", *dataDir, *admin, token)
	return 0
}

func runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprintln(stderr, "usage: cortex agent add --id AGENT [--admin]")
		return 2
	}
	flags := flag.NewFlagSet("agent add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", config.DefaultDataDir(), "Cortex data directory")
	agentID := flags.String("id", "", "agent id")
	admin := flags.Bool("admin", false, "grant review and governance permission")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	token, err := config.AddAgent(*dataDir, *agentID, *admin)
	if err != nil {
		fmt.Fprintf(stderr, "add agent: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "agent=%s\ntoken=%s\n", *agentID, token)
	return 0
}

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", config.DefaultDataDir(), "Cortex data directory")
	listen := flags.String("listen", "", "override configured HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	file, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	address := file.Listen
	if *listen != "" {
		address = *listen
	}
	hub, err := cortex.Open(cortex.Config{
		DatabasePath: config.DatabasePath(*dataDir),
		AdminAgents:  file.AdminAgents,
	})
	if err != nil {
		fmt.Fprintf(stderr, "open Cortex: %v\n", err)
		return 1
	}
	defer hub.Close()
	authenticator, err := config.NewReloadingAuthenticator(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "initialize authenticator: %v\n", err)
		return 1
	}

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.New(hub, authenticator),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Fprintf(stdout, "Cortex listening on http://%s\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "serve Cortex: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Cortex - standalone local memory hub

Usage:
  cortex init [--data-dir DIR] [--admin AGENT] [--listen ADDRESS]
  cortex agent add --id AGENT [--admin] [--data-dir DIR]
  cortex connector sync hermes --home HERMES_HOME [--data-dir DIR]
  cortex import holographic --database MEMORY_STORE_DB --agent AGENT [--project PROJECT]
  cortex serve [--data-dir DIR] [--listen ADDRESS]
  cortex version`)
}
