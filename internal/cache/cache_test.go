package cache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
)

func open(t *testing.T, maxBytes int64) *Cache {
	t.Helper()
	c, err := Open(t.TempDir(), maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPutGetRoundTrip(t *testing.T) {
	c := open(t, 0)
	in := Entry{
		Key: "https://www.sumo-api.com/api/basho/202609", FetchedAt: time.Now().Truncate(time.Second), Pinned: true,
		Validators: httpx.Validators{ETag: `"e1"`, LastModified: "Tue, 13 Oct 2026 08:00:00 GMT"},
		Body:       bytes.Repeat([]byte(`{"x":1}`), 1000),
	}
	if err := c.Put(in); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get(in.Key)
	if !ok || !bytes.Equal(got.Body, in.Body) || got.Validators != in.Validators || !got.Pinned || !got.FetchedAt.Equal(in.FetchedAt) {
		t.Errorf("Get = %+v, %v", got, ok)
	}
	if _, ok := c.Get("https://elsewhere"); ok {
		t.Error("an unknown key was found")
	}
	if c.Size() == 0 || c.Size() > 2000 { // gzip shrinks the repetitive 7 KB body
		t.Errorf("Size = %d", c.Size())
	}
}

func TestTouchKeepsTheBody(t *testing.T) {
	c := open(t, 0)
	if err := c.Put(Entry{Key: "k", FetchedAt: time.Unix(0, 0), Body: []byte("body")}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	v := httpx.Validators{ETag: `"e2"`}
	if err := c.Touch("k", now, v); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get("k")
	if !ok || string(got.Body) != "body" || !got.FetchedAt.Equal(now) || got.Validators != v {
		t.Errorf("after Touch = %+v, %v", got, ok)
	}
	if err := c.Touch("missing", now, v); err == nil {
		t.Error("Touch of a missing key succeeded")
	}
}

func TestUnreadableEntriesAreNotFound(t *testing.T) {
	c := open(t, 0)
	if err := c.Put(Entry{Key: "k", FetchedAt: time.Now(), Body: []byte("b")}); err != nil {
		t.Fatal(err)
	}
	path := c.path("k")
	data, _ := os.ReadFile(path)
	header, body, _ := bytes.Cut(data, []byte{'\n'})

	old := bytes.Replace(header, []byte(`"schema":1`), []byte(`"schema":0`), 1)
	if err := os.WriteFile(path, append(append(old, '\n'), body...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); ok {
		t.Error("an old-schema entry was found")
	}
	if err := os.WriteFile(path, append(append(header, '\n'), "not gzip"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); ok {
		t.Error("an entry with a corrupt body was found")
	}
}

func TestEvictionKeepsPinnedAndRecentlyUsed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := open(t, 4000)           // four entries of ~1070 B exceed it; dropping one fits 90 %
		body := func(i int) []byte { // ~900 B of pseudo-random bytes: gzip cannot shrink them
			var b []byte
			for h := sha256.Sum256([]byte{byte(i)}); len(b) < 900; h = sha256.Sum256(h[:]) {
				b = append(b, h[:]...)
			}
			return b
		}
		put := func(key string, pinned bool, i int) {
			t.Helper()
			if err := c.Put(Entry{Key: key, Pinned: pinned, FetchedAt: time.Now(), Body: body(i)}); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Second) // distinct last-use times
		}
		put("finished", true, 1)
		put("old", false, 2)
		put("used", false, 3)
		c.Get("used") // touched: now more recent than "old"
		time.Sleep(time.Second)
		put("newest", false, 4) // over the limit: evict down to 90 %

		if c.Size() > 3600 {
			t.Errorf("Size = %d, want ≤ 3600 (90 %% of the limit)", c.Size())
		}
		for key, want := range map[string]bool{"finished": true, "old": false, "used": true, "newest": true} {
			if _, ok := c.Get(key); ok != want {
				t.Errorf("%s present = %v, want %v", key, ok, want)
			}
		}
	})
}

func TestSizeSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	c, err := Open(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := c.Put(Entry{Key: fmt.Sprint(i), FetchedAt: time.Now(), Body: []byte("body")}); err != nil {
			t.Fatal(err)
		}
	}
	// Replacing an entry must not double-count it.
	if err := c.Put(Entry{Key: "0", FetchedAt: time.Now(), Body: []byte("body")}); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if again.Size() != c.Size() {
		t.Errorf("size after reopen = %d, before = %d", again.Size(), c.Size())
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	if got, _ := DefaultDir(); got != "/tmp/xdg/basho" {
		t.Errorf("with XDG_CACHE_HOME: %s", got)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	if got, err := DefaultDir(); err != nil || filepath.Base(got) != "basho" {
		t.Errorf("default: %s, %v", got, err)
	}
}
