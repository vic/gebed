// Command gebed serves the beads web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"gebed/internal/mcp"
	"gebed/internal/peer"
	"gebed/internal/registry"
	"gebed/internal/server"
	"gebed/internal/skill"
	"gebed/internal/store"
	"gebed/internal/theme"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		serveMCP(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "skill" {
		installSkill(os.Args[2:])
		return
	}
	addr := flag.String("addr", "127.0.0.1:7878", "listen address (loopback only by default)")
	claude := flag.String("claude", "", "Claude Code config dir (default ~/.claude)")
	flag.Parse()
	cfg, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	reg, err := registry.Load(filepath.Join(cfg, "gebed", "repos.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, dir := range flag.Args() { // repos given on the command line are registered
		name, p := registry.ParseLine(dir)
		if _, err := reg.AddNamed(name, p); err != nil {
			log.Fatal(err)
		}
	}
	themes := func() []theme.Scheme {
		all := theme.Builtin()
		user, errs := theme.LoadDir(os.DirFS(filepath.Join(cfg, "gebed")), "themes")
		for _, e := range errs {
			if !errors.Is(e, fs.ErrNotExist) {
				log.Printf("theme: %v", e)
			}
		}
		return append(all, user...)
	}
	u, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	if *claude == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		*claude = filepath.Join(home, ".claude")
	}
	socks := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "cc-socks")
	net := peer.Net{
		SessDir:  filepath.Join(*claude, "sessions"),
		ProcRoot: "/proc",
		FromSock: filepath.Join(socks, "gebed.sock"),
		FromName: "gebed",
	}
	// Sessions reply through bead comments; the endpoint only gives them a live address.
	switch ep, err := peer.Listen(net.FromSock, func(b []byte) { log.Printf("peer frame: %d bytes", len(b)) }); {
	case err == nil:
		defer func() { _ = ep.Close() }()
	case !errors.Is(err, peer.ErrInUse):
		log.Printf("peer endpoint: %v (messages to sessions still go out)", err)
	}
	log.Printf("gebed on http://%s", *addr)
	srv := server.New(reg, store.Exec, themes, net, u.Username)
	srv.Warm()
	// Guard refuses cross-site writes and DNS-rebinding hosts, so a web page
	// the user visits cannot drive the loopback API.
	log.Fatal(http.ListenAndServe(*addr, server.Guard(*addr, srv)))
}

// installSkill implements `gebed skill install [-to DIR]`.
func installSkill(args []string) {
	fs := flag.NewFlagSet("skill install", flag.ExitOnError)
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	to := fs.String("to", filepath.Join(home, ".claude", "skills"), "skills directory")
	if len(args) == 0 || args[0] != "install" {
		log.Fatal("usage: gebed skill install [-to DIR]")
	}
	if err := fs.Parse(args[1:]); err != nil {
		log.Fatal(err)
	}
	p, err := skill.Install(*to)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("installed %s", p)
}

// serveMCP implements `gebed mcp [-url URL]`: an MCP server on stdio that
// forwards every tool call to a running gebed. Stdout carries only protocol
// messages; logs go to stderr.
func serveMCP(args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	def := os.Getenv("GEBED_URL")
	if def == "" {
		def = "http://127.0.0.1:7878"
	}
	base := fs.String("url", def, "base URL of the running gebed (env GEBED_URL)")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	s := &mcp.Server{Base: *base}
	if err := s.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
