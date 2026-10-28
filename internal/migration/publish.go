package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

// The archive is already verified and protected locally. Copy only its bytes to
// the user's export filesystem, which may not implement Windows ACLs or fsync.
// Close and read back the destination before making the final ZIP visible.
func copyVerifiedArchive(ctx context.Context, source, destination string, progress func(int64, int64)) (err error) {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		if err != nil {
			os.Remove(destination)
		}
	}()
	hash := sha256.New()
	buffer := make([]byte, 1<<20)
	var done int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		var count int
		count, err = src.Read(buffer)
		if count > 0 {
			if _, e := io.MultiWriter(dst, hash).Write(buffer[:count]); e != nil {
				return e
			}
			done += int64(count)
			if progress != nil {
				progress(done, info.Size())
			}
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return err
		}
	}
	if err = dst.Close(); err != nil {
		return err
	}
	readback, e := os.Open(destination)
	if e != nil {
		return e
	}
	defer readback.Close()
	actual := sha256.New()
	if _, err = io.Copy(actual, contextReader{ctx, readback}); err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), actual.Sum(nil)) {
		return errors.New("destination archive checksum does not match the verified local archive")
	}
	return nil
}
