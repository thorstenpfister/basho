package cache

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// Schema is the entry format version; entries of other versions are ignored.
const Schema = 1

// Entry is one cached response. Freshness is not the cache's business: it
// depends on the tournament calendar at read time, so the caller decides from
// FetchedAt (SPEC §7.2).
type Entry struct {
	Key        string           `json:"key"` // e.g. the URL; also aids debugging
	FetchedAt  time.Time        `json:"fetchedAt"`
	Validators httpx.Validators `json:"validators"`
	Pinned     bool             `json:"pinned,omitempty"` // never evicted: finished tournaments
	Schema     int              `json:"schema"`
	Body       []byte           `json:"-"`
}

// Cache is an on-disk response cache: one file per entry, a JSON header line
// followed by the gzipped body. It is safe for concurrent use within one
// process.
type Cache struct {
	dir      string
	maxBytes int64

	mu   sync.Mutex
	size int64 // bytes on disk, kept up to date by Put, Touch and eviction
}

// DefaultMaxBytes is the size limit of SPEC §7.2.
const DefaultMaxBytes = 200 << 20

// DefaultDir is $XDG_CACHE_HOME/basho, or the OS cache directory's basho
// (~/Library/Caches/basho on macOS). XDG_CACHE_HOME is honoured on macOS too,
// so the cache can live with other dotfile-managed paths.
func DefaultDir() (string, error) {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "basho"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "basho"), nil
}

// Open opens (and creates) a cache in dir with a size limit (zero means
// DefaultMaxBytes).
func Open(dir string, maxBytes int64) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	c := &Cache{dir: dir, maxBytes: maxBytes}
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err == nil {
			c.size += info.Size()
		}
		return err
	})
	return c, err
}

// Get returns the entry for key. A missing, unreadable or old-schema entry is
// reported as not found.
func (c *Cache) Get(key string) (Entry, bool) {
	path := c.path(key)
	e, body, err := readFile(path)
	if err != nil || e.Schema != Schema || e.Key != key {
		return Entry{}, false
	}
	if e.Body, err = gunzip(body); err != nil {
		return Entry{}, false
	}
	if !e.Pinned { // last use, for eviction; pinned entries are never evicted
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return e, true
}

// Put stores e, replacing any entry with the same key, and evicts the least
// recently used unpinned entries if the cache grows past its limit.
func (c *Cache) Put(e Entry) error {
	var body bytes.Buffer
	zw := gzip.NewWriter(&body)
	if _, err := zw.Write(e.Body); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return c.write(e, body.Bytes())
}

// Touch records that key was confirmed unchanged (a 304): it updates the
// fetch time and validators without rewriting the body.
func (c *Cache) Touch(key string, fetchedAt time.Time, v httpx.Validators) error {
	e, body, err := readFile(c.path(key))
	if err != nil {
		return err
	}
	e.FetchedAt, e.Validators = fetchedAt, v
	return c.write(e, body)
}

// Size returns the bytes the cache occupies on disk.
func (c *Cache) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// write stores the header line and an already gzipped body.
func (c *Cache) write(e Entry, gzBody []byte) error {
	e.Schema = Schema
	header, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data := append(append(header, '\n'), gzBody...)

	path := c.path(e.Key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	old := fileSize(path)
	if err := atomicfile.Write(path, data); err != nil {
		return err
	}
	// The file's mtime is the last-use time; set it from the clock Get uses.
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		return err
	}
	c.size += int64(len(data)) - old
	if c.size > c.maxBytes {
		return c.evict()
	}
	return nil
}

// evict removes least recently used unpinned entries until the cache is at 90 %
// of its limit, so the next few writes do not trigger another scan. c.mu is held.
func (c *Cache) evict() error {
	type stored struct {
		path     string
		lastUsed time.Time
		size     int64
	}
	var candidates []stored
	err := filepath.WalkDir(c.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if e, _, err := readHeader(path); err == nil && e.Pinned {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		candidates = append(candidates, stored{path, info.ModTime(), info.Size()})
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(candidates, func(a, b stored) int { return a.lastUsed.Compare(b.lastUsed) })
	target := c.maxBytes * 9 / 10
	for _, s := range candidates {
		if c.size <= target {
			break
		}
		if err := os.Remove(s.path); err != nil {
			return err
		}
		c.size -= s.size
	}
	return nil
}

// path spreads entries over 256 directories by the hash of their key.
func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, h[:2], h)
}

// readFile returns an entry's header and its still gzipped body.
func readFile(path string) (Entry, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, nil, err
	}
	header, body, _ := bytes.Cut(data, []byte{'\n'})
	var e Entry
	if err := json.Unmarshal(header, &e); err != nil {
		return Entry{}, nil, err
	}
	return e, body, nil
}

// readHeader reads only the header line.
func readHeader(path string) (Entry, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return Entry{}, nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	line, err := bufio.NewReader(f).ReadBytes('\n')
	if err != nil {
		return Entry{}, nil, err
	}
	var e Entry
	return e, nil, json.Unmarshal(line, &e)
}

func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
