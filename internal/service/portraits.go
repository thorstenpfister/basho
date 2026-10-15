package service

import (
	"image"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
)

// PortraitCredits returns every shipped photo's attribution, by name.
func (s *Service) PortraitCredits() ([]portrait.Credit, error) {
	p, err := s.portraits()
	if err != nil {
		return nil, err
	}
	return p.Credits(), nil
}

// Portrait returns a wrestler's portrait at size (portrait.Sizes) and its
// photo's attribution; false when there is none.
func (s *Service) Portrait(id domain.RikishiID, size int) (*image.Paletted, portrait.Credit, bool) {
	p, err := s.portraits()
	if err != nil {
		return nil, portrait.Credit{}, false
	}
	c, ok := p.Credit(id)
	if !ok {
		return nil, c, false
	}
	img, err := p.Image(id, size)
	return img, c, err == nil
}
