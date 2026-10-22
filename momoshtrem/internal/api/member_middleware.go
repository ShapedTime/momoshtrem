package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// MemberHeader names the household member a request is for (PLAN §6
// "Household members"): the web's proxy sends the picker's choice.
const MemberHeader = "X-Momoshtrem-Member"

// CodeUnknownMember is a 400: the member header names an id no member has.
// The web forgets its choice and opens the picker.
const CodeUnknownMember = "unknown_member"

// memberMiddleware puts the request's member in its context (member.WithID)
// for every /api route but /members, which never reads the header: the
// header's id, or the lowest without one. A value that isn't a positive
// integer is 400 bad_request, an id no member has 400 unknown_member;
// /status then goes on without a member. With nil members it returns nil.
func memberMiddleware(members Members) gin.HandlerFunc {
	if members == nil {
		return nil
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !isAPIPath(path) || isMembersPath(path) {
			c.Next()
			return
		}
		ctx := c.Request.Context()
		id, err := members.Resolve(ctx, c.GetHeader(MemberHeader))
		switch {
		case err == nil:
			c.Request = c.Request.WithContext(member.WithID(ctx, id))
		case errors.Is(err, member.ErrUnknown) && path == statusPath:
			// Status still answers, with no one's Trakt connected.
		case errors.Is(err, member.ErrMalformed):
			errorResponse(c, http.StatusBadRequest, MemberHeader+" must be a member's id.")
			c.Abort()
			return
		case errors.Is(err, member.ErrUnknown):
			errorCodeResponse(c, http.StatusBadRequest, CodeUnknownMember, "Nobody in the household has that id: choose who's watching again.")
			c.Abort()
			return
		case errors.Is(err, context.Canceled):
			errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
			c.Abort()
			return
		default:
			slog.Error("Failed to resolve the household member", "path", path, "error", err)
			errorResponse(c, http.StatusInternalServerError, "Couldn't read the household's members.")
			c.Abort()
			return
		}
		c.Next()
	}
}

const statusPath = "/api/status"

// isMembersPath: /api/members and below.
func isMembersPath(path string) bool {
	return path == "/api/members" || strings.HasPrefix(path, "/api/members/")
}
