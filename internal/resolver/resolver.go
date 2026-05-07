package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
)

type Resolver struct {
	cache *cache.Cache
	api   *api.Client
}

func New(c *cache.Cache, a *api.Client) *Resolver {
	return &Resolver{cache: c, api: a}
}

// ResolveDesign returns a design ID for `query`. Tries cache by ID, then
// cache by exact-title (case-insensitive), then API by listing.
func (r *Resolver) ResolveDesign(query string) (string, error) {
	if d, err := r.cache.FindDesignByID(query); err == nil && d != nil {
		return d.ID, nil
	}
	matches, err := r.cache.FindDesignByName(query)
	if err == nil && len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		return "", ambiguity("design", matches[0].ID, matches[1].ID)
	}
	if r.api == nil {
		return "", &NotFoundError{Resource: "design", Query: query}
	}
	var hit *string
	multi := []string{}
	err = r.api.ListDesigns(context.Background(), func(d api.Design) error {
		if d.ID == query || strings.EqualFold(d.Title, query) {
			id := d.ID
			if hit == nil {
				hit = &id
			} else {
				multi = append(multi, id)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if hit == nil {
		return "", &NotFoundError{Resource: "design", Query: query}
	}
	if len(multi) > 0 {
		return "", ambiguity("design", append([]string{*hit}, multi...)...)
	}
	return *hit, nil
}

// ResolveTemplate is the same shape, against templates.
func (r *Resolver) ResolveTemplate(query string) (string, error) {
	if t, err := r.cache.FindTemplateByID(query); err == nil && t != nil {
		return t.ID, nil
	}
	matches, err := r.cache.FindTemplateByName(query)
	if err == nil && len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		return "", ambiguity("template", matches[0].ID, matches[1].ID)
	}
	if r.api == nil {
		return "", &NotFoundError{Resource: "template", Query: query}
	}
	var hit *string
	multi := []string{}
	err = r.api.ListTemplates(context.Background(), func(t api.BrandTemplate) error {
		if t.ID == query || strings.EqualFold(t.Title, query) {
			id := t.ID
			if hit == nil {
				hit = &id
			} else {
				multi = append(multi, id)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if hit == nil {
		return "", &NotFoundError{Resource: "template", Query: query}
	}
	if len(multi) > 0 {
		return "", ambiguity("template", append([]string{*hit}, multi...)...)
	}
	return *hit, nil
}

type NotFoundError struct {
	Resource string
	Query    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Resource, e.Query)
}

type AmbiguityError struct {
	Resource    string
	Suggestions []string
}

func (e *AmbiguityError) Error() string {
	return fmt.Sprintf("%s name matched multiple: %v", e.Resource, e.Suggestions)
}

func ambiguity(resource string, ids ...string) error {
	return &AmbiguityError{Resource: resource, Suggestions: ids}
}
