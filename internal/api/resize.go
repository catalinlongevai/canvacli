package api

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ResizePreset is one of the four Canva-defined preset names accepted by
// POST /resizes when design_type.type == "preset". Per the OpenAPI
// PresetDesignTypeName enum these are the ONLY four names — anything else
// (instagram_post, a4_document, etc.) must go through the custom branch
// below with explicit width/height.
type ResizePreset string

const (
	ResizePresetDoc          ResizePreset = "doc"
	ResizePresetEmail        ResizePreset = "email"
	ResizePresetPresentation ResizePreset = "presentation"
	ResizePresetWhiteboard   ResizePreset = "whiteboard"
)

// IsValidResizePreset reports whether s is one of the four Canva preset
// names. Returns false for "" (empty).
func IsValidResizePreset(s string) bool {
	switch ResizePreset(s) {
	case ResizePresetDoc, ResizePresetEmail, ResizePresetPresentation, ResizePresetWhiteboard:
		return true
	}
	return false
}

// resizeDesignType is the discriminated union used in the request body.
// Only one of preset/custom is populated per call.
type resizeDesignType struct {
	Type   string `json:"type"`             // "preset" or "custom"
	Name   string `json:"name,omitempty"`   // when Type == "preset"
	Width  int    `json:"width,omitempty"`  // when Type == "custom"
	Height int    `json:"height,omitempty"` // when Type == "custom"
}

// resizeRequestBody is what the Canva API actually expects on the wire.
type resizeRequestBody struct {
	DesignID   string           `json:"design_id"`
	DesignType resizeDesignType `json:"design_type"`
}

// ResizeRequest is the convenience input used by callers. Provide either
// Preset (one of the four Canva-defined names) OR Width+Height for custom
// dimensions. Setting both is an error.
type ResizeRequest struct {
	DesignID string
	Preset   ResizePreset
	Width    int
	Height   int
}

// resizeJob mirrors the entire job body returned by GET /resizes/{id} on
// success. Unlike exports/autofill (which inline result fields onto job),
// resize nests its payload under `result`. PollJob decodes T from the
// whole job object, so we mirror the shape exactly here.
type resizeJob struct {
	Result ResizeResult `json:"result"`
}

// ResizeResult is the contents of job.result on a successful resize.
// trial_information is included on free-tier accounts when applicable.
type ResizeResult struct {
	Design           Design            `json:"design"`
	TrialInformation *TrialInformation `json:"trial_information,omitempty"`
}

// TrialInformation is returned for free-tier users who have a small resize
// quota outside Canva Pro. uses_remaining drops to 0 when the trial is
// exhausted; subsequent calls return the trial_quota_exceeded error code.
type TrialInformation struct {
	UsesRemaining int    `json:"uses_remaining"`
	UpgradeURL    string `json:"upgrade_url"`
}

// ResizeDesign submits a POST /resizes job and polls until terminal status.
// Always creates a NEW design — the original is untouched.
//
// Endpoint: POST /v1/resizes (NOT /designs/{id}/resize as one might guess
// from REST convention). Required scopes: design:content:read AND
// design:content:write. Capability: resize (Canva Pro or trial).
func (c *Client) ResizeDesign(ctx context.Context, req ResizeRequest) (*ResizeResult, error) {
	hasPreset := req.Preset != ""
	hasCustom := req.Width > 0 || req.Height > 0
	if hasPreset && hasCustom {
		return nil, errors.New("resize: specify either Preset or Width/Height, not both")
	}
	if !hasPreset && !hasCustom {
		return nil, errors.New("resize: must specify Preset or Width/Height")
	}

	body := resizeRequestBody{DesignID: req.DesignID}
	if hasPreset {
		if !IsValidResizePreset(string(req.Preset)) {
			return nil, errors.New("resize: invalid preset (allowed: doc, email, presentation, whiteboard)")
		}
		body.DesignType = resizeDesignType{Type: "preset", Name: string(req.Preset)}
	} else {
		if req.Width < 40 || req.Width > 8000 || req.Height < 40 || req.Height > 8000 {
			return nil, errors.New("resize: width and height must each be between 40 and 8000 pixels")
		}
		body.DesignType = resizeDesignType{Type: "custom", Width: req.Width, Height: req.Height}
	}

	var submit struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/resizes", body, &submit); err != nil {
		return nil, err
	}
	job, err := PollJob[resizeJob](ctx, c, "/resizes/"+submit.Job.ID, PollOptions{
		Initial: 500 * time.Millisecond,
		Max:     5 * time.Second,
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &job.Result, nil
}
