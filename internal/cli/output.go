package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func writeOutput(ctx context.Context, root *os.Root, rel, display string, audio io.Reader) (string, string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return display, "", err
	}
	tmp := filepath.Join(filepath.Dir(rel), ".music-unlock-"+hex.EncodeToString(id[:])+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return display, "", err
	}
	defer root.Remove(tmp)
	defer f.Close()
	_, err = io.CopyBuffer(f, &contextReader{ctx, audio}, make([]byte, 128*1024))
	if err != nil {
		return display, "", err
	}
	if err := f.Sync(); err != nil {
		return display, "", err
	}
	if err := f.Close(); err != nil {
		return display, "", err
	}
	if err := ctx.Err(); err != nil {
		return display, "", err
	}
	// Hard-link publishes a completed file without replacing an existing name,
	// unlike os.Rename on Unix. A competing process can only cause a skip.
	if err := root.Link(tmp, rel); err != nil {
		if errors.Is(err, os.ErrExist) {
			return display, "跳过", nil
		}
		// FAT/exFAT and some network volumes have no hard links. Publish using
		// exclusive creation, retaining the same never-overwrite guarantee.
		return copyExclusive(ctx, root, tmp, rel, display)
	}
	return display, "成功", nil
}

func copyExclusive(ctx context.Context, root *os.Root, tmp, rel, display string) (string, string, error) {
	in, err := root.Open(tmp)
	if err != nil {
		return display, "", err
	}
	defer in.Close()
	out, err := root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return display, "跳过", nil
	}
	if err != nil {
		return display, "", err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			root.Remove(rel)
		}
	}()
	if _, err := io.CopyBuffer(out, &contextReader{ctx, in}, make([]byte, 128*1024)); err != nil {
		return display, "", err
	}
	if err := out.Sync(); err != nil {
		return display, "", err
	}
	if err := out.Close(); err != nil {
		return display, "", err
	}
	ok = true
	return display, "成功", nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
