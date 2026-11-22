package rule

import (
	"fmt"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/rule/match"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

func GetAllRules(ctx *context.Context, userId int) []*dto.Rule {
	var res []*models.Rule
	var err error
	if userId == 0 {
		return nil
	} else {
		err = db.Instance.Where("user_id=?", userId).Decr("sort").Find(&res)
	}

	if err != nil {
		log.WithContext(ctx).Errorf("sqlERror :%v", err)
	}
	var ret []*dto.Rule
	for _, rule := range res {
		ret = append(ret, (&dto.Rule{}).Decode(rule))
	}

	return ret
}

func MatchRule(ctx *context.Context, rule *dto.Rule, email *parsemail.Email) bool {

	if rule == nil || email == nil {
		return false
	}
	for _, r := range rule.Rules {
		if r == nil {
			return false
		}
		switch r.Field {
		case "From", "Subject", "To", "Cc", "Text", "Html", "Content":
		default:
			return false
		}
		var m match.Match

		switch r.Type {
		case match.RuleTypeRegex:
			m = match.NewRegexMatch(r.Field, r.Rule)
		case match.RuleTypeContains:
			m = match.NewContainsMatch(r.Field, r.Rule)
		case match.RuleTypeEq:
			m = match.NewEqualMatch(r.Field, r.Rule)
		}
		if m == nil {
			return false
		}

		if !m.Match(ctx, email) {
			return false
		}
	}

	return true
}

func DoRule(ctx *context.Context, rule *dto.Rule, email *parsemail.Email, user *models.User, rawEmailData []byte) {
	log.WithContext(ctx).Debugf("执行规则:%s", rule.Name)

	switch rule.Action {
	case dto.READ:
		if email.MessageId > 0 {
			_, err := db.Instance.Table(&models.UserEmail{}).Where("email_id=? and user_id=?", email.MessageId, rule.UserId).Cols("is_read").Update(map[string]interface{}{"is_read": 1})
			if err != nil {
				log.WithContext(ctx).Errorf("sqlERror :%v", err)
			}
		}
	case dto.DELETE:
		_, err := db.Instance.Table(&models.UserEmail{}).Where("email_id=? and user_id=?", email.MessageId, rule.UserId).Cols("status").Update(map[string]interface{}{"status": consts.EmailStatusDel})
		if err != nil {
			log.WithContext(ctx).Errorf("sqlERror :%v", err)
		}
	case dto.FORWARD:
		for _, target := range forward.Targets(rule.Params) {
			if err := forward.EnqueueForUser(ctx, user.ID, email, rawEmailData, target); err != nil {
				log.WithContext(ctx).Errorf("Forward Error:%v", err)
			} else {
				log.WithContext(ctx).Infof("Forward Success:%s@%s -> %s", user.Account, domain.DefaultName(), target)
			}
		}
	case dto.MOVE:
		doMove(ctx, rule, email, user)
	}

}

func doForward(ctx *context.Context, email *parsemail.Email, forwardAddress string, user *models.User, rawEmailData []byte) error {
	targets := forward.Targets(forwardAddress)
	if len(targets) == 0 {
		return fmt.Errorf("invalid forward address: %s", forwardAddress)
	}
	if len(rawEmailData) == 0 && email != nil && email.From != nil {
		var err error
		rawEmailData, err = email.BuildBytes(ctx, false)
		if err != nil {
			return err
		}
	}
	for _, target := range targets {
		if err := forward.EnqueueForUser(ctx, user.ID, email, rawEmailData, target); err != nil {
			return err
		}
	}
	return nil
}

func forwardToLocalUser(ctx *context.Context, email *parsemail.Email, account, forwardAddress string) error {
	if email.MessageId <= 0 {
		return fmt.Errorf("email has not been saved before local forwarding")
	}

	var user models.User
	has, err := db.Instance.Table(&models.User{}).
		Where("LOWER(account)=LOWER(?) and disabled=0", account).
		Get(&user)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("local forward user not found: %s", forwardAddress)
	}

	var ue models.UserEmail
	has, err = db.Instance.Table(&models.UserEmail{}).
		Where("email_id=? and user_id=?", email.MessageId, user.ID).
		Get(&ue)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	_, err = db.Instance.Insert(&models.UserEmail{
		UserID:  user.ID,
		EmailID: cast.ToInt(email.MessageId),
		Status:  cast.ToInt8(email.Status),
	})
	return err
}

func isLocalDomain(domainName string) bool {
	return domain.IsLocal(domainName)
}

func doMove(ctx *context.Context, r *dto.Rule, email *parsemail.Email, user *models.User) {
	if user == nil || email == nil || email.MessageId <= 0 || r.UserId != user.ID {
		return
	}
	owner := *ctx
	owner.UserID = user.ID
	id := cast.ToInt(r.Params)
	if !group.ValidMoveTarget(&owner, id) {
		log.WithContext(ctx).Warn("ignoring mail rule with invalid destination group")
		return
	}
	if name, exists := models.GroupCodeToName[id]; exists {
		if err := group.Move2DefaultBox(&owner, []int{int(email.MessageId)}, name); err != nil {
			log.WithContext(ctx).Warnf("rule group move: %v", err)
		}
	} else {
		group.MoveMailToGroup(&owner, []int{int(email.MessageId)}, id)
	}
}
