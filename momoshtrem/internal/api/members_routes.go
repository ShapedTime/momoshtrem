package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// /members' 409s (PLAN §6 "Household members").
const (
	CodeMemberLimit     = "member_limit"      // the household has six
	CodeMemberNameTaken = "member_name_taken" // another member has the name, ignoring case
	CodeLastMember      = "last_member"       // the household can't be left with nobody
)

// Members are the household's members (PLAN §6 "Household members"):
// /members, and the member each request is for (memberMiddleware).
// *member.Service implements it.
type Members interface {
	// Resolve is the member a request's header names: the lowest id
	// without one, member.ErrMalformed or member.ErrUnknown.
	Resolve(ctx context.Context, header string) (int64, error)
	List(ctx context.Context) ([]member.View, error)
	Create(ctx context.Context, name string, color *string) (member.View, error)
	Update(ctx context.Context, id int64, p member.Patch) (member.View, error)
	Delete(ctx context.Context, id int64) error
}

var _ Members = (*member.Service)(nil)

// registerMemberRoutes adds /members. Without Options.Members they answer
// 503.
func (s *Server) registerMemberRoutes(api *gin.RouterGroup) {
	api.GET("/members", s.listMembers)
	api.POST("/members", s.createMember)
	api.PATCH("/members/:id", s.updateMember)
	api.DELETE("/members/:id", s.deleteMember)
}

func (s *Server) membersReady(c *gin.Context) bool {
	if s.opts.Members == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Household members aren't available.")
		return false
	}
	return true
}

// GET /members → {items: [Member]}, by id, never empty
func (s *Server) listMembers(c *gin.Context) {
	if !s.membersReady(c) {
		return
	}
	items, err := s.opts.Members.List(c.Request.Context())
	if err != nil {
		membersErrorResponse(c, err)
		return
	}
	if items == nil {
		items = []member.View{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// POST /members {name, color?} → 201 Member. Without a color it's the
// first no member uses; unknown fields are ignored.
func (s *Server) createMember(c *gin.Context) {
	if !s.membersReady(c) {
		return
	}
	name, color, ok := readMemberBody(c)
	if !ok {
		return
	}
	if name == nil {
		name = new(string) // the service answers 422 for it
	}
	m, err := s.opts.Members.Create(c.Request.Context(), *name, color)
	if err != nil {
		membersErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

// PATCH /members/{id} with any of {name, color} → 200 Member. Fields left
// out keep their value and unknown ones are ignored. Checks, in order: 400
// (the id, the body), 422, 404, 409.
func (s *Server) updateMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.membersReady(c) {
		return
	}
	var p member.Patch
	if p.Name, p.Color, ok = readMemberBody(c); !ok {
		return
	}
	m, err := s.opts.Members.Update(c.Request.Context(), id, p)
	if err != nil {
		membersErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

// DELETE /members/{id} → 204. It lets go of the member's Trakt, picks and
// Ask, and deletes what's theirs; the household's data stays.
func (s *Server) deleteMember(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.membersReady(c) {
		return
	}
	if err := s.opts.Members.Delete(c.Request.Context(), id); err != nil {
		membersErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// readMemberBody reads a POST or PATCH body: a JSON object whose name and
// color, each nil when it isn't sent, are strings. Anything else is 400, a
// wrong type (null included) too, as the quality profile does; unknown
// fields are ignored. The member service checks the values (422).
func readMemberBody(c *gin.Context) (name, color *string, ok bool) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return nil, nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		errorResponse(c, http.StatusBadRequest, "Send the member as a JSON object.")
		return nil, nil, false
	}
	for _, f := range []struct {
		key string
		dst **string
	}{{"name", &name}, {"color", &color}} {
		value, sent := fields[f.key]
		if !sent {
			continue
		}
		var v string
		if string(value) == "null" || json.Unmarshal(value, &v) != nil {
			errorResponse(c, http.StatusBadRequest, f.key+" must be a string.")
			return nil, nil, false
		}
		*f.dst = &v
	}
	return name, color, true
}

// membersErrorResponse maps the members' errors.
func membersErrorResponse(c *gin.Context, err error) {
	var invalid *member.ValidationError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusUnprocessableEntity, ErrorBody{Error: ErrorDetail{Code: CodeValidationError, Message: invalid.Message, Field: invalid.Field}})
	case errors.Is(err, member.ErrNotFound):
		errorResponse(c, http.StatusNotFound, "Nobody in the household has that id.")
	case errors.Is(err, member.ErrLimit):
		errorCodeResponse(c, http.StatusConflict, CodeMemberLimit, "A household can have up to six people.")
	case errors.Is(err, member.ErrNameTaken):
		errorCodeResponse(c, http.StatusConflict, CodeMemberNameTaken, "Someone in the household already has that name.")
	case errors.Is(err, member.ErrLastMember):
		errorCodeResponse(c, http.StatusConflict, CodeLastMember, "The household needs at least one person.")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Household members request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read or change the household's members.")
	}
}
