package httpapi

import (
	"net/http"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// グループの経路。仕様は docs/specs/2026-10-11-groups-design.md「API」。
//
// メンバーでない人・無いグループ・使えないリンクは、どれも同じ 404 と
// 同じ本文になる（apperror.ErrGroupNotFound）。ここでは分けない。

type groupSummaryDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	MyName      string `json:"my_name"`
}

type groupsResponse struct {
	Groups []groupSummaryDTO `json:"groups"`
}

type createGroupRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type createdGroupResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	InviteToken string `json:"invite_token"`
}

type memberDayDTO struct {
	Date      string   `json:"date"`
	Exercises []string `json:"exercises"`
}

type memberWeekDTO struct {
	DisplayName string         `json:"display_name"`
	IsMe        bool           `json:"is_me"`
	Days        []memberDayDTO `json:"days"`
}

type groupWeekResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	InviteToken string          `json:"invite_token"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	Members     []memberWeekDTO `json:"members"`
}

type inviteTokenResponse struct {
	InviteToken string `json:"invite_token"`
}

type invitePreviewResponse struct {
	GroupID       string `json:"group_id"`
	Name          string `json:"name"`
	MemberCount   int    `json:"member_count"`
	AlreadyMember bool   `json:"already_member"`
}

type joinGroupRequest struct {
	DisplayName string `json:"display_name"`
}

type joinedGroupResponse struct {
	GroupID string `json:"group_id"`
}

func (h *Handler) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	gs, err := h.groups.Mine(r.Context(), user)
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]groupSummaryDTO, 0, len(gs))
	for _, g := range gs {
		out = append(out, groupSummaryDTO{ID: g.ID.String(), Name: g.Name, MemberCount: g.MemberCount, MyName: g.MyName})
	}
	writeJSON(w, http.StatusOK, groupsResponse{Groups: out})
}

func (h *Handler) handlePostGroup(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req createGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	g, err := h.createGroup.Execute(r.Context(), user, usecase.CreateGroupInput{
		Name: req.Name, DisplayName: req.DisplayName,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createdGroupResponse{
		ID: g.ID().String(), Name: g.Name().String(), InviteToken: g.Invite().String(),
	})
}

func (h *Handler) handleGetGroupWeek(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	raw := r.URL.Query().Get("date")
	if raw == "" {
		respondError(w, invalidInput("date クエリパラメータが必要である"))
		return
	}
	date, err := training.ParseDate(raw)
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}

	week, err := h.groups.Week(r.Context(), user, r.PathValue("id"), date)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupWeekResponse(week))
}

func toGroupWeekResponse(week query.GroupWeek) groupWeekResponse {
	out := groupWeekResponse{
		ID: week.ID.String(), Name: week.Name, InviteToken: week.Invite.String(),
		From: week.From.String(), To: week.To.String(), Members: make([]memberWeekDTO, 0, len(week.Members)),
	}
	for _, m := range week.Members {
		mw := memberWeekDTO{DisplayName: m.DisplayName, IsMe: m.IsMe, Days: make([]memberDayDTO, 0, len(m.Days))}
		for _, d := range m.Days {
			mw.Days = append(mw.Days, memberDayDTO{Date: d.Date.String(), Exercises: d.Exercises})
		}
		out.Members = append(out.Members, mw)
	}
	return out
}

func (h *Handler) handlePostGroupInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	token, err := h.rotateInvite.Execute(r.Context(), user, r.PathValue("id"))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inviteTokenResponse{InviteToken: token.String()})
}

func (h *Handler) handleDeleteGroupMembership(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if err := h.leaveGroup.Execute(r.Context(), user, r.PathValue("id")); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	p, err := h.groups.Invite(r.Context(), user, r.PathValue("token"))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invitePreviewResponse{
		GroupID: p.ID.String(), Name: p.Name, MemberCount: p.MemberCount, AlreadyMember: p.AlreadyMember,
	})
}

func (h *Handler) handlePostInviteJoin(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req joinGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	id, err := h.joinGroup.Execute(r.Context(), user, r.PathValue("token"), req.DisplayName)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, joinedGroupResponse{GroupID: id.String()})
}
