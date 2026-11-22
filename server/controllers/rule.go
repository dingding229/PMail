package controllers

import (
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/rule"
	"github.com/Jinnrry/pmail/services/rule/match"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	log "github.com/sirupsen/logrus"
	"net/http"
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

	for _, r := range data.Rules {
		if r == nil || !array.InArray(r.Field, []string{"From", "Subject", "To", "Cc", "Text", "Html", "Content"}) {
			response.Localized(ctx, response.ParamsError, "rule_invalid_field").FPrint(w)
			return
		}
		if r.Type != match.RuleTypeRegex && r.Type != match.RuleTypeContains && r.Type != match.RuleTypeEq {
			response.Localized(ctx, response.ParamsError, "rule_invalid_match").FPrint(w)
			return
		}
	}

	if err := save(ctx, data.Encode()); err != nil {
		log.WithContext(ctx).Errorf("Save rule: %v", err)
		response.Localized(ctx, response.ServerError, "rule_write_failed").FPrint(w)
		return
	}
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)
}

func save(ctx *context.Context, p *models.Rule) error {

	if p.Id > 0 {
		_, err := db.Instance.Exec(db.WithContext(ctx, "update rule set name=? ,value = ? ,action = ?,params = ?,sort = ? where id = ? and user_id = ?"), p.Name, p.Value, p.Action, p.Params, p.Sort, p.Id, ctx.UserID)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	} else {
		_, err := db.Instance.Exec(db.WithContext(ctx, "insert into rule (name,value,user_id,action,params,sort) values (?,?,?,?,?,?)"), p.Name, p.Value, ctx.UserID, p.Action, p.Params, p.Sort)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

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
