package sumoapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

// DefaultBaseURL is sumo-api.com's API root.
const DefaultBaseURL = "https://www.sumo-api.com"

// pageSize is the largest page /rikishis returns (V4).
const pageSize = 1000

// Client reads sumo-api.com.
type Client struct {
	http httpx.Getter
	base string
}

// New returns a Client for base (DefaultBaseURL in production).
func New(g httpx.Getter, base string) *Client {
	return &Client{http: g, base: base}
}

// Basho returns a tournament's metadata. A tournament sumo-api does not have
// yet (banzuke unpublished, or before 1960) is source.ErrNotYetAvailable.
func (c *Client) Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error) {
	var dto bashoDTO
	if err := c.getJSON(ctx, "/api/basho/"+id.String(), &dto); err != nil {
		return domain.Basho{}, err
	}
	if dto.Date == "" {
		return domain.Basho{}, fmt.Errorf("basho %s: %w", id, source.ErrNotYetAvailable)
	}
	return mapBasho(id, dto), nil
}

// Banzuke returns a division's banzuke with each wrestler's 15 day results.
func (c *Client) Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	var dto banzukeDTO
	if err := c.getJSON(ctx, fmt.Sprintf("/api/basho/%s/banzuke/%s", id, div), &dto); err != nil {
		return nil, err
	}
	if len(dto.East)+len(dto.West) == 0 {
		return nil, fmt.Errorf("banzuke %s %s: %w", id, div, source.ErrNotYetAvailable)
	}
	return mapBanzuke(dto)
}

// Torikumi returns a division's bouts of one day, in match order.
func (c *Client) Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	var dto torikumiDTO
	if err := c.getJSON(ctx, fmt.Sprintf("/api/basho/%s/torikumi/%s/%d", id, div, day), &dto); err != nil {
		return nil, err
	}
	if len(dto.Torikumi) == 0 {
		return nil, fmt.Errorf("torikumi %s %s day %d: %w", id, div, day, source.ErrNotYetAvailable)
	}
	return mapTorikumi(id, div, dto)
}

// RikishiIndex returns every wrestler, retired ones included, page by page.
func (c *Client) RikishiIndex(ctx context.Context) ([]domain.Rikishi, error) {
	var all []domain.Rikishi
	for skip := 0; ; skip += pageSize {
		var page rikishiPageDTO
		path := fmt.Sprintf("/api/rikishis?intai=true&limit=%d&skip=%d", pageSize, skip)
		if err := c.getJSON(ctx, path, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Records {
			all = append(all, mapRikishi(r))
		}
		if len(page.Records) < pageSize || len(all) >= page.Total {
			return all, nil
		}
	}
}

// Measurements returns every wrestler's height and weight at a tournament.
func (c *Client) Measurements(ctx context.Context, id domain.BashoID) (map[domain.RikishiID]domain.Measurement, error) {
	var dto []measurementDTO
	if err := c.getJSON(ctx, "/api/measurements?bashoId="+id.String(), &dto); err != nil {
		return nil, err
	}
	m := make(map[domain.RikishiID]domain.Measurement, len(dto))
	for _, d := range dto {
		m[domain.RikishiID(d.RikishiID)] = domain.Measurement{Height: d.Height, Weight: d.Weight}
	}
	return m, nil
}

// getJSON fetches path and decodes it into v, mapping failures to the
// source errors.
func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.http.Get(ctx, c.base+path, httpx.Validators{})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %w: %w", path, source.ErrUpstream, err)
	}
	switch {
	case resp.OK():
	case resp.Status == http.StatusNotFound:
		return fmt.Errorf("%s: %w", path, source.ErrNotFound)
	case resp.Status == http.StatusTooManyRequests:
		return fmt.Errorf("%s: %w", path, source.ErrRateLimited)
	default:
		return fmt.Errorf("%s: HTTP %d: %w", path, resp.Status, source.ErrUpstream)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		return fmt.Errorf("%s: %w: %w", path, source.ErrUpstream, err)
	}
	return nil
}
