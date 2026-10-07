package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSQLiteFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not enforced by Windows chmod")
	}
	directory := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(directory, "usage.sqlite")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{directory, path} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s is accessible to other users: %o", filepath.Base(name), info.Mode().Perm())
		}
	}
	if err = os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("existing database permissions were not restricted: %v", err)
	}
}
