package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// AISettings serves the AI settings and their usage (PLAN §6
// "Recommendations (R1–R3)"; the AI core, #154): app_settings key "ai",
// shared with the classifier. Both work without ANTHROPIC_API_KEY.
type AISettings interface {
	// AISettings reads the document over library.DefaultAISettings.
	AISettings(ctx context.Context) (recommend.Settings, error)
	// UpdateAISettings applies the fields set in one jsonb `||` statement
	// and answers every setting.
	UpdateAISettings(ctx context.Context, patch recommend.SettingsPatch) (recommend.Settings, error)
	// AIUsage is this server-local month's spend, budget, runs and asks.
	AIUsage(ctx context.Context) (*recommend.UsageReport, error)
}

// registerAISettingsRoutes adds GET/PUT /settings/ai and GET
// /settings/ai/usage. Without Options.AISettings they answer 503.
func (s *Server) registerAISettingsRoutes(api *gin.RouterGroup) {
	api.GET("/settings/ai", s.getAISettings)
	api.PUT("/settings/ai", s.putAISettings)
	api.GET("/settings/ai/usage", s.getAIUsage)
}

func (s *Server) aiSettingsReady(c *gin.Context) bool {
	if s.opts.AISettings == nil {
		errorResponse(c, http.StatusServiceUnavailable, "AI settings aren't available")
		return false
	}
	return true
}

// GET /settings/ai → Settings
func (s *Server) getAISettings(c *gin.Context) {
	if !s.aiSettingsReady(c) {
		return
	}
	settings, err := s.opts.AISettings.AISettings(c.Request.Context())
	if err != nil {
		aiSettingsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

// PUT /settings/ai with any of the settings → 200 Settings (all of them).
// Fields left out keep their value and unknown ones are ignored; a body
// that isn't a JSON object is 400, a bad value 422 naming the field.
func (s *Server) putAISettings(c *gin.Context) {
	if !s.aiSettingsReady(c) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return
	}
	patch, err := parseAISettingsPatch(raw)
	if err != nil {
		aiSettingsErrorResponse(c, err)
		return
	}
	settings, err := s.opts.AISettings.UpdateAISettings(c.Request.Context(), patch)
	if err != nil {
		aiSettingsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

// GET /settings/ai/usage → UsageReport
func (s *Server) getAIUsage(c *gin.Context) {
	if !s.aiSettingsReady(c) {
		return
	}
	usage, err := s.opts.AISettings.AIUsage(c.Request.Context())
	if err == nil && usage == nil {
		err = errors.New("no usage report")
	}
	if err != nil {
		aiSettingsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, emptyUsageSlices(*usage))
}

var errNotAnObject = errors.New("send the settings to change as a JSON object")

// aiSettingsFields are PUT /settings/ai's fields, checked in this order.
var aiSettingsFields = []string{
	"rec_enabled", "rec_model", "rec_schedule", "monthly_budget_usd",
	"classifier_enabled", "classifier_model", "auto_apply_high_confidence",
}

// parseAISettingsPatch reads the fields sent: errNotAnObject for a body
// that isn't a JSON object, *recommend.FieldError for the first bad value.
func parseAISettingsPatch(raw []byte) (recommend.SettingsPatch, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return recommend.SettingsPatch{}, errNotAnObject
	}
	var p recommend.SettingsPatch
	for _, name := range aiSettingsFields {
		value, sent := fields[name]
		if !sent {
			continue
		}
		var err *recommend.FieldError
		switch name {
		case "rec_enabled":
			p.RecEnabled, err = boolField(name, value)
		case "classifier_enabled":
			p.ClassifierEnabled, err = boolField(name, value)
		case "auto_apply_high_confidence":
			p.AutoApplyHighConfidence, err = boolField(name, value)
		case "rec_model":
			p.RecModel, err = oneOfField(name, value, recommend.RecModels)
		case "rec_schedule":
			p.RecSchedule, err = oneOfField(name, value, []string{recommend.ScheduleNightly, recommend.ScheduleWeekly, recommend.ScheduleManual})
		case "classifier_model":
			p.ClassifierModel, err = oneOfField(name, value, recommend.ClassifierModels)
		case "monthly_budget_usd":
			p.SetBudget = true
			p.MonthlyBudgetUSD, err = budgetField(name, value)
		}
		if err != nil {
			return recommend.SettingsPatch{}, err
		}
	}
	return p, nil
}

func boolField(name string, raw json.RawMessage) (*bool, *recommend.FieldError) {
	var v bool
	if string(raw) == "null" || json.Unmarshal(raw, &v) != nil {
		return nil, &recommend.FieldError{Field: name, Message: name + " must be true or false"}
	}
	return &v, nil
}

func oneOfField(name string, raw json.RawMessage, allowed []string) (*string, *recommend.FieldError) {
	var v string
	if json.Unmarshal(raw, &v) != nil || !slices.Contains(allowed, v) {
		last := len(allowed) - 1
		return nil, &recommend.FieldError{Field: name, Message: name + " must be " + strings.Join(allowed[:last], ", ") + " or " + allowed[last]}
	}
	return &v, nil
}

// budgetField: null is no cap; otherwise a number of 0 or more.
func budgetField(name string, raw json.RawMessage) (*float64, *recommend.FieldError) {
	if string(raw) == "null" {
		return nil, nil
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return nil, &recommend.FieldError{Field: name, Message: name + " must be 0 or more, or null for no cap"}
	}
	return &v, nil
}

func aiSettingsErrorResponse(c *gin.Context, err error) {
	var field *recommend.FieldError
	switch {
	case errors.As(err, &field):
		c.JSON(http.StatusUnprocessableEntity, ErrorBody{Error: ErrorDetail{Code: CodeValidationError, Message: field.Message, Field: field.Field}})
	case errors.Is(err, errNotAnObject):
		errorResponse(c, http.StatusBadRequest, "Send the settings to change as a JSON object.")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("AI settings request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read or save the AI settings.")
	}
}
