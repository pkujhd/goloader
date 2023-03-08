package jit

import (
	"fmt"
	"github.com/bmatcuk/doublestar/v4"
	"io"
	"os"
	"path/filepath"
)

func Copy(src, dest string, skipPattern []string) error {
	for _, shouldSkip := range skipPattern {
		matched, err := doublestar.Match(shouldSkip, src)
		if matched {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to check filepath match during Copy(): %w", err)
		}
	}

	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		err = copyDir(src, dest, skipPattern)
	} else if srcInfo.Mode()&os.ModeSymlink != 0 {
		linkSrc, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("failed to follow symlink during Copy(): %w", err)
		}
		if !filepath.IsAbs(linkSrc) {
			linkSrc = filepath.Join(filepath.Dir(src), linkSrc)
		}
		err = Copy(linkSrc, dest, skipPattern)
	} else {
		err = copyFile(src, dest)
	}
	return err
}

func copyFile(src, dst string) (err error) {
	if err = os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return
	}

	f, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		err2 := f.Close()
		if err == nil {
			err = err2
		}
	}()

	s, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() {
		err2 := s.Close()
		if err == nil {
			err = err2
		}
	}()

	if _, err = io.Copy(f, s); err != nil {
		return err
	}

	return
}

func copyDir(srcDir, dstDir string, skipPattern []string) (err error) {
	dirFiles, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}

	for _, entry := range dirFiles {
		cs, cd := filepath.Join(srcDir, entry.Name()), filepath.Join(dstDir, entry.Name())

		if err = Copy(cs, cd, skipPattern); err != nil {
			return
		}
	}

	return
}
