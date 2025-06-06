package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gopherguides/hype"
	"github.com/markbates/cleo"
	"github.com/markbates/plugins"
)

var _ plugins.Needer = &Marked{}

type Marked struct {
	cleo.Cmd

	// a folder containing all chapters of a book, for example
	ContextPath string
	File        string        // optional file name to preview
	Timeout     time.Duration // default: 5s
	Parser      *hype.Parser  // If nil, a default parser is used.
	ParseOnly   bool          // if true, only parse the file and exit
	Section     int           // default: 1
	Verbose     bool          // default: false

	flags *flag.FlagSet

	mu sync.RWMutex
}

func (cmd *Marked) WithPlugins(fn plugins.FeederFn) error {
	if cmd == nil {
		return fmt.Errorf("marked is nil")
	}

	if fn == nil {
		return fmt.Errorf("fn is nil")
	}

	cmd.mu.Lock()
	defer cmd.mu.Unlock()

	cmd.Feeder = fn

	return nil
}

func (cmd *Marked) ScopedPlugins() plugins.Plugins {
	if cmd == nil {
		return nil
	}

	type marker interface {
		MarkedPlugin()
	}

	plugs := cmd.Cmd.ScopedPlugins()

	res := make(plugins.Plugins, 0, len(plugs))
	for _, p := range plugs {
		if _, ok := p.(marker); ok {
			res = append(res, p)
		}
	}

	return res
}

func (cmd *Marked) SetParser(p *hype.Parser) error {
	if cmd == nil {
		return fmt.Errorf("marked is nil")
	}

	cmd.mu.Lock()
	defer cmd.mu.Unlock()

	cmd.Parser = p
	return nil
}

func (cmd *Marked) Flags() (*flag.FlagSet, error) {
	if err := cmd.validate(); err != nil {
		return nil, err
	}

	cmd.mu.Lock()
	defer cmd.mu.Unlock()

	if cmd.flags != nil {
		return cmd.flags, nil
	}

	cmd.flags = flag.NewFlagSet("marked", flag.ContinueOnError)
	cmd.flags.SetOutput(io.Discard)
	cmd.flags.BoolVar(&cmd.ParseOnly, "p", cmd.ParseOnly, "if true, only parse the file and exit")
	cmd.flags.DurationVar(&cmd.Timeout, "timeout", DefaultTimeout, "timeout for execution")
	cmd.flags.StringVar(&cmd.ContextPath, "context", cmd.ContextPath, "a folder containing all chapters of a book, for example")
	cmd.flags.StringVar(&cmd.File, "f", cmd.File, "optional file name to preview, if not provided, defaults to hype.md")
	cmd.flags.IntVar(&cmd.Section, "section", 0, "")
	cmd.flags.BoolVar(&cmd.Verbose, "v", false, "enable verbose output for debugging")

	return cmd.flags, nil
}

func (cmd *Marked) Main(ctx context.Context, pwd string, args []string) error {
	err := cmd.main(ctx, pwd, args)
	if err == nil {
		return nil
	}

	cmd.mu.Lock()
	to := cmd.Timeout
	if to == 0 {
		to = DefaultTimeout
		cmd.Timeout = to
	}
	cmd.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	var mu sync.Mutex

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	return err
}

func (cmd *Marked) main(ctx context.Context, pwd string, args []string) error {
	if err := cmd.validate(); err != nil {
		return err
	}

	mp := os.Getenv("MARKED_PATH")

	pwd = filepath.Dir(mp)

	if err := (&cmd.Cmd).Init(); err != nil {
		return err
	}

	flags, err := cmd.Flags()
	if err != nil {
		return err
	}

	if err := flags.Parse(args); err != nil {
		return err
	}
	if cmd.Verbose {
		// enable debugging
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	return WithTimeout(ctx, cmd.Timeout, func(ctx context.Context) error {
		// TODO Document what this does
		if mo, ok := os.LookupEnv("MARKED_ORIGIN"); ok {
			pwd = mo
		}

		return WithinDir(pwd, func() error {
			return cmd.execute(ctx, pwd)
		})
	})
}

func (cmd *Marked) execute(ctx context.Context, pwd string) error {
	err := cmd.validate()
	if err != nil {
		return err
	}

	if cmd.FS == nil {
		cmd.FS = os.DirFS(pwd)
	}

	// TODO Document what this does
	mp := os.Getenv("MARKED_PATH")

	slog.Debug("execute", "pwd", pwd, "file", cmd.File, "MARKED_PATH", mp)
	p := cmd.Parser

	if p == nil {
		p = hype.NewParser(cmd.FS)
	}

	if p.Section == 0 {
		p.Section = 1
	}

	if cmd.Section > 0 {
		p.Section = cmd.Section
	}

	p.Root = filepath.Dir(mp)
	p.Filename = filepath.Base(mp)

	if len(cmd.File) > 0 {
		f, err := cmd.FS.Open(cmd.File)
		if err != nil {
			return err
		}
		defer f.Close()

		cmd.IO.In = f
	}

	doc, err := p.Parse(cmd.Stdin())
	if err != nil {
		return err
	}

	if !cmd.ParseOnly {
		if err := doc.Execute(ctx); err != nil {
			return err
		}
	}

	pages, err := doc.Pages()
	if err != nil {
		return err
	}

	for i, page := range pages {
		if i+1 == len(pages) {
			break
		}

		page.Nodes = append(page.Nodes, hype.Text("\n<!--BREAK-->\n"))
	}

	fmt.Fprintln(cmd.Stdout(), doc.String())

	return nil

}

func (cmd *Marked) validate() error {
	if cmd == nil {
		return fmt.Errorf("cmd is nil")
	}

	cmd.mu.Lock()
	defer cmd.mu.Unlock()

	if cmd.Timeout == 0 {
		cmd.Timeout = DefaultTimeout
	}

	return nil
}
