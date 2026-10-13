// Package fixture maps API requests to recorded files under testdata/ and
// serves them back in tests, so recorder and tests share one naming rule.
package fixture

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/thorstenpfister/basho/internal/httpx"
)

var unsafe = strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "-")

// Name returns the file name a request path is recorded under, e.g.
// "/api/basho/202609/torikumi/Makuuchi/15" → "basho_202609_torikumi_Makuuchi_15.json".
func Name(path string) string {
	path = strings.TrimPrefix(path, "/api/v1/") // osada.us
	path = strings.TrimPrefix(path, "/api/")    // sumo-api.com
	return unsafe.Replace(strings.TrimSuffix(path, ".json")) + ".json"
}

// Getter serves recorded files from dir for requests under base. A request
// without a recording is a 404, as the real API would answer.
type Getter struct {
	Dir  string
	Base string
}

// Get implements httpx.Getter.
func (g Getter) Get(_ context.Context, url string, _ httpx.Validators) (*httpx.Response, error) {
	path, ok := strings.CutPrefix(url, g.Base)
	if !ok {
		return nil, errors.New("fixture: " + url + " is not under " + g.Base)
	}
	body, err := os.ReadFile(filepath.Join(g.Dir, Name(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return &httpx.Response{Status: 404}, nil
	}
	if err != nil {
		return nil, err
	}
	return &httpx.Response{Status: 200, Body: body}, nil
}
