package controllers

import (
	stderrors "errors"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/rule"
	"github.com/Jinnrry/pmail/services/rule/match"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"net/http"
	"xorm.io/xorm"
)

func GetRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	res := rule.GetAllRules(ctx, ctx.UserID)
	response.NewSuccessResponse(res).FPrint(w)
}

func UpsertRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {

	var data *dto.Rule
	if !readRequest(ctx, w, req, &data, false) {
		return
	}
	if data == nil || data.Id < 0 {
		response.Localized(ctx, response.ParamsError, "rule_invalid").FPrint(w)
		return
	}
	if data.Action != dto.READ && data.Action != dto.FORWARD && data.Action != dto.DELETE && data.Action != dto.MOVE {
		response.Localized(ctx, response.ParamsError, "rule_invalid_action").FPrint(w)
		return
	}

	if data.Action == dto.FORWARD {
		targets := forward.Targets(data.Params)
		if len(targets) == 0 {
			response.Localized(ctx, response.ParamsError, "forward_invalid_target").FPrint(w)
			return
		}
		if _, err := forward.ValidateTargets(ctx.UserID, targets); err != nil {
			response.Localized(ctx, response.ParamsError, forwardErrorKey(err)).FPrint(w)
			return
		}
	}

	if len(data.Rules) > 32 {
		apiError(ctx, w, 400, response.ParamsError, "rule_count_limit")
		return
	}
	if data.Action == dto.MOVE && !group.ValidMoveTarget(ctx, cast.ToInt(data.Params)) {
		apiError(ctx, w, 400, response.ParamsError, "group_invalid_parent_or_target")
		return
	}
	for _, r := range data.Rules {
		if r == nil || !array.InArray(r.Field, []string{"From", "Subject", "To", "Cc", "Text", "Html", "Content"}) {
			response.Localized(ctx, response.ParamsError, "rule_invalid_field").FPrint(w)
			return
		}
		if r.Type != match.RuleTypeRegex && r.Type != match.RuleTypeContains && r.Type != match.RuleTypeEq {
			response.Localized(ctx, response.ParamsError, "rule_invalid_match").FPrint(w)
			return
		}
		if r.Type == match.RuleTypeRegex {
			if _, err := match.Compile(r.Rule); err != nil {
				apiError(ctx, w, 400, response.ParamsError, "rule_invalid_regex")
				return
			}
		}
	}

	if err := save(ctx, data.Encode()); err != nil {
		if stderrors.Is(err, errTooManyRules) {
			apiError(ctx, w, 400, response.ParamsError, "rule_count_limit")
			return
		}
		log.WithContext(ctx).Errorf("Save rule: %v", err)
		response.Localized(ctx, response.ServerError, "rule_write_failed").FPrint(w)
		return
	}
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)
}

const maxUserRules = 100

var errTooManyRules = stderrors.New("too many mail rules")

func save(ctx *context.Context, p *models.Rule) error {

	if p.Id > 0 {
		_, err := db.Instance.Exec(db.WithContext(ctx, "update rule set name=? ,value = ? ,action = ?,params = ?,sort = ? where id = ? and user_id = ?"), p.Name, p.Value, p.Action, p.Params, p.Sort, p.Id, ctx.UserID)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		q := tx.ID(ctx.UserID).Cols("id")
		if db.Instance.DriverName() != "sqlite" {
			q.ForUpdate()
		}
		var owner models.User
		ok, err := q.Get(&owner)
		if err != nil {
			return err
		}
		if !ok {
			return stderrors.New("rule owner no longer exists")
		}
		n, err := tx.Where("user_id=?", ctx.UserID).Count(&models.Rule{})
		if err != nil {
			return err
		}
		if n >= maxUserRules {
			return errTooManyRules
		}
		row := *p
		row.UserId = ctx.UserID
		_, err = tx.Insert(&row)
		return err
	})
}

type delRuleReq struct {
	Id int `json:"id"`
}

func DelRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var data delRuleReq
	if !readRequest(ctx, w, req, &data, false) {
		return
	}

	if data.Id <= 0 {
		response.Localized(ctx, response.ParamsError, "invalid_id").FPrint(w)
		return
	}

	_, err := db.Instance.Exec(db.WithContext(ctx, "delete from rule where id =? and user_id =?"), data.Id, ctx.UserID)
	if err != nil {
		log.WithContext(ctx).Errorf("Delete rule: %v", err)
		response.Localized(ctx, response.ServerError, "rule_delete_failed").FPrint(w)
		return
	}

	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)
}
