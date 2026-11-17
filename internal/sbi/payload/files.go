package payload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var errInvalidPayloadPath = errors.New("invalid staged payload path")

// DirectoryResolver reads only regular files below a Host-created private root.
// The Host must prevent untrusted concurrent mutation of this root.
type DirectoryResolver struct{ root string }

func NewDirectoryResolver(root string) (*DirectoryResolver, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errInvalidPayloadPath
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, errInvalidPayloadPath
	}
	return &DirectoryResolver{root: resolved}, nil
}

func (r *DirectoryResolver) Open(ctx context.Context, relative string) (io.ReadCloser, int64, error) {
	ctx = contextOrBackground(ctx)
	if r == nil {
		return nil, 0, errInvalidPayloadPath
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	target, parts, err := confinedPath(r.root, relative)
	if err != nil {
		return nil, 0, err
	}
	var finalInfo os.FileInfo
	for index := range parts {
		candidate := filepath.Join(r.root, filepath.Join(parts[:index+1]...))
		info, err := os.Lstat(candidate)
		if err != nil {
			return nil, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, 0, errInvalidPayloadPath
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, 0, errInvalidPayloadPath
		}
		finalInfo = info
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(finalInfo, info) {
		file.Close()
		return nil, 0, errInvalidPayloadPath
	}
	return &exactSizeReadCloser{ctx: ctx, ReadCloser: file, remaining: info.Size()}, info.Size(), nil
}

// DirectorySink stores response attachments under generated names, never remote filenames.
type DirectorySink struct {
	root   string
	limits Limits
	mu     sync.Mutex
	next   int
	total  int64
}

func NewDirectorySink(root string, limits Limits) (*DirectorySink, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errInvalidPayloadPath
	}
	limits, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, errInvalidPayloadPath
	}
	return &DirectorySink{root: resolved, limits: limits}, nil
}

func (s *DirectorySink) Store(
	ctx context.Context,
	contentID string,
	contentType string,
	source io.Reader,
	maximum int64,
) (Attachment, error) {
	ctx = contextOrBackground(ctx)
	if s == nil || source == nil {
		return Attachment{}, ErrInvalidPayload
	}
	if err := ctx.Err(); err != nil {
		return Attachment{}, err
	}
	if _, _, err := parseMediaType(contentType); err != nil {
		return Attachment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	remaining := s.limits.MaxTotalBytes - s.total
	if maximum < 0 {
		return Attachment{}, ErrPayloadTooLarge
	}
	if maximum > s.limits.MaxBinaryBytes {
		maximum = s.limits.MaxBinaryBytes
	}
	if remaining < maximum {
		maximum = remaining
	}
	if maximum < 0 {
		return Attachment{}, ErrPayloadTooLarge
	}

	s.next++
	name := fmt.Sprintf("part-%04d.bin", s.next)
	target, _, err := confinedPath(s.root, name)
	if err != nil {
		return Attachment{}, err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Attachment{}, err
	}

	hash := sha256.New()
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: source}, maximum+1)
	size, copyErr := io.Copy(io.MultiWriter(file, hash), limited)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || size > maximum {
		_ = os.Remove(target)
		if ctx.Err() != nil {
			return Attachment{}, ctx.Err()
		}
		if size > maximum {
			return Attachment{}, ErrPayloadTooLarge
		}
		if copyErr != nil {
			return Attachment{}, copyErr
		}
		return Attachment{}, closeErr
	}
	s.total += size
	return Attachment{
		ContentID: contentID, RelativePath: name, ContentType: contentType,
		Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func confinedPath(root, relative string) (string, []string, error) {
	if relative == "" || strings.ContainsAny(relative, "\\:") || strings.HasPrefix(relative, "/") {
		return "", nil, errInvalidPayloadPath
	}
	clean := path.Clean(relative)
	if clean == "." || clean != relative || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", nil, errInvalidPayloadPath
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", nil, errInvalidPayloadPath
		}
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	resolved, err := filepath.Rel(root, target)
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(os.PathSeparator)) {
		return "", nil, errInvalidPayloadPath
	}
	return target, parts, nil
}
