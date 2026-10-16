package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// QualityProfiles reads and saves the quality profile (PLAN §6 "Search").
// *library.SettingsRepository implements it.
type QualityProfiles interface {
	QualityProfile(ctx context.Context) (library.QualityProfile, error)
	// SetQualityProfile validates and stores the profile, answering the
	// saved one; an invalid field matches library.ErrInvalidQualityProfile.
	SetQualityProfile(ctx context.Context, p library.QualityProfile) (library.QualityProfile, error)
}

var _ QualityProfiles = (*library.SettingsRepository)(nil)

// registerQualityRoutes adds GET and PUT /settings/quality-profile, served
// from Options.Settings. Without it they answer 503.
func (s *Server) registerQualityRoutes(api *gin.RouterGroup) {
	var profiles QualityProfiles
	if s.opts.Settings != nil { // a nil pointer in the interface wouldn't read as nil
		profiles = s.opts.Settings
	}
	registerQualityProfileRoutes(api, profiles)
}

func registerQualityProfileRoutes(api *gin.RouterGroup, profiles QualityProfiles) {
	h := qualityHandlers{profiles: profiles}
	api.GET("/settings/quality-profile", h.get)
	api.PUT("/settings/quality-profile", h.put)
}

type qualityHandlers struct {
	profiles QualityProfiles // nil: 503
}

func (h qualityHandlers) ready(c *gin.Context) bool {
	if h.profiles == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Settings aren't available")
		return false
	}
	return true
}

// GET /settings/quality-profile
func (h qualityHandlers) get(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	profile, err := h.profiles.QualityProfile(c.Request.Context())
	if err != nil {
		slog.Error("Failed to read the quality profile", "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read the quality profile")
		return
	}
	c.JSON(http.StatusOK, profile)
}

// PUT /settings/quality-profile takes the whole profile and answers the
// saved one; a missing field takes its default. 422 names an invalid field.
func (h qualityHandlers) put(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	profile := library.DefaultQualityProfile()
	if err := c.ShouldBindJSON(&profile); err != nil {
		msg := "The body must be a quality profile"
		if typeErr := (*json.UnmarshalTypeError)(nil); errors.As(err, &typeErr) && typeErr.Field != "" {
			msg = typeErr.Field + " has the wrong type"
		}
		errorResponse(c, http.StatusBadRequest, msg)
		return
	}
	saved, err := h.profiles.SetQualityProfile(c.Request.Context(), profile)
	switch {
	case errors.Is(err, library.ErrInvalidQualityProfile):
		errorResponse(c, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		slog.Error("Failed to save the quality profile", "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't save the quality profile")
	default:
		c.JSON(http.StatusOK, saved)
	}
}
