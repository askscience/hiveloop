package main

// Encrypted backups, as subcommands of the hiveloop binary (nothing else to install on the server):
//
//	hiveloop backup-keygen                         new key pair (keep the secret part off the server)
//	hiveloop backup  -data DIR -r RECIPIENT -o F   consistent snapshot of DIR, encrypted to RECIPIENT
//	hiveloop restore -i IDENTITY_FILE -o DIR F     decrypt F and unpack it into DIR
//
// The snapshot holds hiveloop.db (copied with VACUUM INTO, safe while the app runs)
// plus files/ (users' documents). Format: tar.gz encrypted with age (https://age-encryption.org).

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

// runSubcommand handles "hiveloop <subcommand>"; returns false for a normal server start.
func runSubcommand() bool {
	if len(os.Args) < 2 {
		return false
	}
	var err error
	switch os.Args[1] {
	case "backup":
		err = cmdBackup(os.Args[2:])
	case "restore":
		err = cmdRestore(os.Args[2:])
	case "backup-keygen":
		err = cmdKeygen()
	default:
		return false
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	return true
}

func cmdKeygen() error {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return err
	}
	fmt.Printf("# Hiveloop backup key — created %s\n# public key (recipient): %s\n%s\n", time.Now().Format(time.RFC3339), id.Recipient(), id)
	return nil
}

func cmdBackup(args []string) error {
	fl := flag.NewFlagSet("backup", flag.ExitOnError)
	data := fl.String("data", "./data", "data directory")
	rcp := fl.String("r", "", "age recipient (public key, age1...)")
	out := fl.String("o", "", "output file")
	fl.Parse(args)
	if *rcp == "" || *out == "" {
		return errors.New("usage: hiveloop backup -data DIR -r age1... -o FILE")
	}
	r, err := age.ParseX25519Recipient(strings.TrimSpace(*rcp))
	if err != nil {
		return fmt.Errorf("recipient: %w", err)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(*out), ".hiveloop-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, db := range []string{"hiveloop.db"} {
		src := filepath.Join(*data, db)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := vacuumInto(src, filepath.Join(tmp, db)); err != nil {
			return fmt.Errorf("%s: %w", db, err)
		}
	}

	f, err := os.OpenFile(*out+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	enc, err := age.Encrypt(f, r)
	if err != nil {
		f.Close()
		return err
	}
	gz := gzip.NewWriter(enc)
	tw := tar.NewWriter(gz)
	add := func(base, rel string) error {
		return filepath.WalkDir(filepath.Join(base, rel), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() && !d.IsDir() {
				return nil
			}
			name, _ := filepath.Rel(base, p)
			hdr, _ := tar.FileInfoHeader(info, "")
			hdr.Name = filepath.ToSlash(name)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			src, err := os.Open(p)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(tw, src)
			return err
		})
	}
	for _, db := range []string{"hiveloop.db"} {
		if _, err := os.Stat(filepath.Join(tmp, db)); err == nil {
			if err := add(tmp, db); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(filepath.Join(*data, "files")); err == nil {
		if err := add(*data, "files"); err != nil {
			return err
		}
	}
	for _, c := range []io.Closer{tw, gz, enc, f} {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return os.Rename(*out+".part", *out)
}

// vacuumInto writes a consistent copy of a live SQLite database.
func vacuumInto(src, dst string) error {
	db, err := sql.Open("sqlite", "file:"+src+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`VACUUM INTO ?`, dst)
	return err
}

func cmdRestore(args []string) error {
	fl := flag.NewFlagSet("restore", flag.ExitOnError)
	idf := fl.String("i", "", "identity file (secret key)")
	out := fl.String("o", "./restored", "directory to unpack into")
	fl.Parse(args)
	if *idf == "" || fl.NArg() != 1 {
		return errors.New("usage: hiveloop restore -i KEY_FILE -o DIR BACKUP.age")
	}
	kf, err := os.Open(*idf)
	if err != nil {
		return err
	}
	ids, err := age.ParseIdentities(kf)
	kf.Close()
	if err != nil {
		return err
	}
	in, err := os.Open(fl.Arg(0))
	if err != nil {
		return err
	}
	defer in.Close()
	dec, err := age.Decrypt(in, ids...)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(dec)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	root, _ := filepath.Abs(*out)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dst := filepath.Join(root, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(dst, root+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in backup: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(dst, 0o750)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(dst), 0o750)
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			n++
		}
	}
	fmt.Printf("restored %d files into %s\n", n, root)
	return nil
}
