package detail

import (
	"fmt"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)
import "xorm.io/builder"

func GetEmailDetail(ctx *context.Context, id int, markRead bool) (*response.EmailResponseData, error) {
	return getDetail(ctx, id, markRead, false)
}

func WebDetail(ctx *context.Context, id int, markRead bool) (*response.EmailResponseData, error) {
	return getDetail(ctx, id, markRead, true)
}

func getDetail(ctx *context.Context, id int, markRead, web bool) (*response.EmailResponseData, error) {
	// 先查是否是本人的邮件
	var ue models.UserEmail
	_, err := db.Instance.Where("email_id = ? AND user_id = ?", id, ctx.UserID).Asc("id").Get(&ue)
	if err != nil {
		return nil, err
	}
	if ue.ID == 0 && (!ctx.IsAdmin || os.Getenv("PMAIL_ADMIN_MAIL_ACCESS") != "1") {
		return nil, errors.New("Not authorized")
	}

	//获取邮件内容
	var email response.EmailResponseData
	cols := "*,1 AS is_read"
	if web {
		cols = "id,type,status,subject,reply_to,from_name,from_address," + db.Instance.Quote("to") + ",bcc,cc,text,html,sender,spf_check,dkim_check,cron_send_time,update_time,send_user_id,size,error,send_date,create_time,msg_id,text_format,auth_results,inbound_analysis,auth_unchecked,projection_ready,CASE WHEN projection_ready THEN '' ELSE attachments END AS attachments"
	}
	found, err := db.Instance.Select(cols).Table("email").Where("id=?", id).Get(&email)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL error:%+v", err)
		return nil, err
	}

	if !found {
		return nil, errors.New("message not found")
	}

	if ue.ID == 0 && !auth.AdminMailOverride(ctx, &email.Email) {
		return nil, errors.New("Not authorized")
	}
	if web {
		if err := mailstore.LoadMetadata([]*models.Email{&email.Email}); err != nil {
			return nil, err
		}
	}
	email.InboundSpam = ue.InboundSpam
	email.IsRead = ue.IsRead
	email.IsStarred = ue.IsStarred
	email.MailboxStatus = ue.Status

	if markRead && ue.ID != 0 && ue.IsRead == 0 {
		err = SetFlags(ctx, []int{id}, boolPtr(true), nil)
		if err != nil {
			log.WithContext(ctx).Errorf("SQL error:%+v", err)
		} else {
			email.IsRead = 1
		}
	}

	return &email, nil
}

func boolPtr(value bool) *bool { return &value }

func MakeRead(ctx *context.Context, emailID int, read bool) {
	if err := SetFlags(ctx, []int{emailID}, &read, nil); err != nil {
		log.WithContext(ctx).Errorf("Set seen: %v", err)
	}
}

// SetFlags updates only the caller's mailbox links, including any copies.
func SetFlags(ctx *context.Context, ids []int, read, starred *bool) error {
	values := map[string]any{}
	if read != nil {
		if *read {
			values["is_read"] = 1
		} else {
			values["is_read"] = 0
		}
	}
	if starred != nil {
		values["is_starred"] = *starred
	}
	if ctx.UserID <= 0 || len(ids) == 0 || len(values) == 0 {
		return nil
	}
	var tests []string
	var args []any
	if read != nil {
		tests = append(tests, "(is_read IS NULL OR is_read<>?)")
		args = append(args, values["is_read"])
	}
	if starred != nil {
		tests = append(tests, "is_starred<>?")
		args = append(args, *starred)
	}
	changed := false
	// Each batch commits independently; a later error must not hide an
	// earlier successful write from other tabs.
	defer func() {
		if changed {
			mailevents.Publish(ctx.UserID)
		}
	}()
	for start := 0; start < len(ids); start += 500 {
		n, err := db.Instance.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).Where("("+strings.Join(tests, " OR ")+")", args...).In("email_id", ids[start:min(start+500, len(ids))]).Update(values)
		if err != nil {
			return err
		}
		changed = changed || n > 0
	}
	return nil
}

func FindUE(ctx *context.Context, groupName string, req list.ImapListReq, uid bool) []models.UserEmail {
	var ue []models.UserEmail
	if uid {
		err := db.Instance.Where(builder.Eq{"id": req.UidList}).Find(&ue)
		if err != nil {
			log.WithContext(ctx).Errorf("SQL error:%+v", err)
		}
		return ue
	} else {
		sql := fmt.Sprintf("SELECT id,email_id, is_read from (SELECT id,email_id, is_read, ROW_NUMBER() OVER (ORDER BY id) AS serial_number FROM `user_email` WHERE (user_id = ? and status = ?)) a WHERE serial_number in (%s)",
			array.Join(req.UidList, ","),
		)
		switch groupName {
		case "INBOX":
			db.Instance.SQL(sql, ctx.UserID, 0).Find(&ue)
		case "Sent Messages":
			db.Instance.SQL(sql, ctx.UserID, 1).Find(&ue)
		case "Drafts":
			db.Instance.SQL(sql, ctx.UserID, 4).Find(&ue)
		case "Deleted Messages":
			db.Instance.SQL(sql, ctx.UserID, 3).Find(&ue)
		case "Junk":
			db.Instance.SQL(sql, ctx.UserID, 5).Find(&ue)
		default:
			groupNames := strings.Split(groupName, "/")
			groupName = groupNames[len(groupNames)-1]

			var group models.Group
			db.Instance.Table("group").Where("user_id=? and name=?", ctx.UserID, groupName).Get(&group)
			if group.ID == 0 {
				return nil
			}
			db.Instance.
				SQL(fmt.Sprintf(
					"SELECT * from (SELECT id,email_id, is_read, ROW_NUMBER() OVER (ORDER BY id) AS serial_number FROM `user_email` WHERE (user_id = ? and group_id = ?)) a WHERE serial_number in (%s)",
					array.Join(req.UidList, ","),
				)).
				Find(&ue, ctx.UserID, group.ID)
		}

		return ue

	}
}
