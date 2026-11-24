package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func GetUserGroupList(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	defaultGroup := []*models.Group{
		{ID: models.INBOX, Name: i18n.T(ctx, "inbox"), FullPath: "/"},     // 收件箱
		{ID: models.Junk, Name: i18n.T(ctx, "junk"), FullPath: "/"},       //垃圾邮件
		{ID: models.Deleted, Name: i18n.T(ctx, "deleted"), FullPath: "/"}, // 已删除
	}

	infos := group.GetGroupList(ctx)

	response.NewSuccessResponse(append(defaultGroup, infos...)).FPrint(w)
}

func GetUserGroup(ctx *context.Context, w http.ResponseWriter, req *http.Request) {

	var body struct {
		Address json.RawMessage `json:"address"`
	}
	if req.Method == http.MethodPost {
		d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 8<<10))
		if err := d.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			response.Localized(ctx, response.ParamsError, "mail_address_invalid").FPrintStatus(w, http.StatusBadRequest)
			return
		}
	}
	value, err := mailaddress.Value(req.URL.Query().Get("address"), body.Address)
	var selected mailaddress.Filter
	if err == nil {
		selected, err = mailaddress.Validate(ctx.UserID, value)
	}
	switch {
	case errors.Is(err, mailaddress.ErrForbidden):
		response.Localized(ctx, response.NoAccessPrivileges, "mail_address_forbidden").FPrintStatus(w, http.StatusForbidden)
		return
	case errors.Is(err, mailaddress.ErrInvalid):
		response.Localized(ctx, response.ParamsError, "mail_address_invalid").FPrintStatus(w, http.StatusBadRequest)
		return
	case err != nil:
		response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
		return
	}

	retData := []*group.GroupItem{
		{
			Label: i18n.T(ctx, "inbox"),
			Tag:   dto.SearchTag{Type: 0, Status: -1, GroupId: 0}.ToString(),
		},
		{
			Label: i18n.T(ctx, "starred"),
			Tag:   dto.SearchTag{Type: -1, Status: -1, GroupId: -1, Starred: true}.ToString(),
		},
		{
			Label: i18n.T(ctx, "outbox"),
			Tag:   dto.SearchTag{Type: 1, Status: -1}.ToString(),
		},
		{
			Label: i18n.T(ctx, "sketch"),
			Tag:   dto.SearchTag{Type: 0, Status: 4}.ToString(),
		},
		{
			Label: i18n.T(ctx, "junk"),
			Tag:   dto.SearchTag{Type: -1, Status: 5}.ToString(),
		},
		{
			Label: i18n.T(ctx, "deleted"),
			Tag:   dto.SearchTag{Type: -1, Status: 3}.ToString(),
		},
	}

	retData = array.Merge(retData, group.GetGroupInfoList(ctx))

	var fill func([]*group.GroupItem) error
	fill = func(items []*group.GroupItem) error {
		for _, item := range items {
			var tag dto.SearchTag
			if err := json.Unmarshal([]byte(item.Tag), &tag); err != nil {
				return err
			}
			var err error
			item.Total, item.Unread, err = list.FolderCount(ctx, tag, selected)
			if err != nil {
				return err
			}
			if err := fill(item.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := fill(retData); err != nil {
		response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
		return
	}

	response.NewSuccessResponse(retData).FPrint(w)
}

type addGroupRequest struct {
	Name     string `json:"name"`
	ParentId int    `json:"parent_id"`
}

func AddGroup(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData addGroupRequest
	if !readRequest(ctx, w, req, &reqData, false) {
		return
	}
	if strings.TrimSpace(reqData.Name) == "" || reqData.ParentId < 0 {
		response.Localized(ctx, response.ParamsError, "group_create_failed").FPrint(w)
		return
	}
	newGroup, err := group.CreateGroup(ctx, reqData.Name, reqData.ParentId)

	if err != nil {
		if errors.Is(err, group.ErrInvalidParent) {
			apiError(ctx, w, 400, response.ParamsError, "group_invalid_parent_or_target")
			return
		}
		if errors.Is(err, group.ErrInvalidName) {
			apiError(ctx, w, 400, response.ParamsError, "group_create_failed")
			return
		}
		if db.IsUniqueViolation(err) {
			serviceError(ctx, w, err)
			return
		}
		log.WithContext(ctx).Errorf("AddGroup: %v", err)
		response.Localized(ctx, response.ServerError, "group_create_failed").FPrint(w)
		return
	}

	response.NewSuccessResponse(newGroup.ID).FPrint(w)
}

type delGroupRequest struct {
	Id int `json:"id"`
}

func DelGroup(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData delGroupRequest
	if !readRequest(ctx, w, req, &reqData, false) {
		return
	}
	if reqData.Id <= 0 {
		response.Localized(ctx, response.ParamsError, "group_delete_failed").FPrint(w)
		return
	}
	succ, err := group.DelGroup(ctx, reqData.Id)

	if err != nil {
		log.WithContext(ctx).Errorf("DelGroup: %v", err)
		response.Localized(ctx, response.ServerError, "group_delete_failed").FPrint(w)
		return
	}
	response.NewSuccessResponse(succ).FPrint(w)
}
