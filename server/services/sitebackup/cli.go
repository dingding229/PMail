package sitebackup

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
)

func CLI(args []string, version string) error {
	if len(args) == 0 {
		return errors.New("usage: pmail backup export|restore (use --help for flags)")
	}
	mode := args[0]
	fs := flag.NewFlagSet("pmail backup "+mode, flag.ContinueOnError)
	passFile := fs.String("passphrase-file", "", "read the passphrase from a file rather than the command line")
	var path *string
	var opt *bool
	switch mode {
	case "export":
		path = fs.String("out", "", "new encrypted archive path")
		opt = fs.Bool("no-tls", false, "omit TLS key and certificate files")
	case "restore":
		path = fs.String("in", "", "encrypted input archive path")
		opt = fs.Bool("yes", false, "replace all current data")
	default:
		return ErrInvalid
	}
	if err := fs.Parse(args[1:]); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return ErrInvalid
	}
	if *path == "" || *passFile == "" || fs.NArg() != 0 {
		return ErrInvalid
	}
	if mode == "restore" && !*opt {
		return ErrConfirm
	}
	f, err := os.OpenFile(*passFile, os.O_RDONLY|noFollow(), 0)
	if err != nil {
		return ErrPassphrase
	}
	if i, e := f.Stat(); e != nil || !i.Mode().IsRegular() {
		f.Close()
		return ErrPassphrase
	}
	data, err := io.ReadAll(io.LimitReader(f, 1027))
	f.Close()
	if err != nil || len(data) > 1026 {
		return ErrPassphrase
	}
	pass := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	clear(data)
	if err = ValidatePassphrase(pass); err != nil {
		return err
	}
	guard, err := Acquire(config.ROOT_PATH)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err = config.Init(); err != nil {
		return err
	}
	engine, err := db.Open(config.Instance, false)
	if err != nil {
		return err
	}
	defer engine.Close()
	s := Store{engine, config.ROOT_PATH, version, config.Instance.SecretKey}
	ctx := context.Background()
	if mode == "export" {
		return Publish(filepath.Dir(*path), filepath.Base(*path), func(out *os.File) error { return s.Export(ctx, out, pass, !*opt, nil) })
	}
	in, err := os.OpenFile(*path, os.O_RDONLY|noFollow(), 0)
	if err != nil {
		return ErrNotFound
	}
	defer in.Close()
	if i, e := in.Stat(); e != nil || !i.Mode().IsRegular() {
		return ErrInvalid
	}
	// An offline destination may be a brand-new database; verify the full
	// archive before allowing SyncSchema to create missing empty tables.
	if err = s.Inspect(ctx, in, pass); err != nil {
		return err
	}
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = db.SyncSchema(engine); err != nil {
		return err
	}
	dir, err := DataDir(s.Root, "backups")
	if err != nil {
		return err
	}
	return s.Restore(ctx, in, pass, func() (func(bool), error) {
		return nil, Publish(dir, Name(s.Version, true), func(out *os.File) error { return s.Export(ctx, out, pass, true, nil) })
	}, nil)
}
