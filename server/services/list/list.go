package list

import (
	"strings"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/utils/context"
)

func countSQLFor(ctx *context.Context, tagInfo dto.SearchTag, keyword string, filters ...mailaddress.Filter) (string, []any) {
	var predicate string
	var addressArgs []any
	if len(filters) != 0 {
		predicate, addressArgs = filters[0].Predicate()
	}
	sqlParams := []any{ctx.UserID}
	sql := `select count(1) from email e left join user_email ue on e.id=ue.email_id where ue.user_id = ? `

	if tagInfo.Starred {
		// IMAP COPY can create multiple mailbox links for one email. Use
		// one live link per email in the virtual Starred folder.
		sql += ` and ue.is_starred = ? and ue.status != 3
			and not exists (select 1 from user_email older where older.user_id=ue.user_id
			and older.email_id=ue.email_id and older.is_starred=ue.is_starred
			and older.status != 3 and older.id < ue.id `
		sqlParams = append(sqlParams, true)
		if predicate != "" {
			sql += " AND (" + strings.ReplaceAll(predicate, "ue.", "older.") + ")"
			sqlParams = append(sqlParams, addressArgs...)
		}
		sql += ")"
	} else if tagInfo.Status != -1 {
		switch tagInfo.Status {
		case consts.EmailStatusDel:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Deleted)
		case consts.EmailStatusDrafts:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Drafts)
		case consts.EmailStatusJunk:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Junk)
		}

	} else if tagInfo.Status == -1 {
		if tagInfo.GroupId > 0 && tagInfo.GroupId < models.INBOX {
			// Custom folders hold whatever the user moved there, including
			// sent mail (status 1), so only hide deleted copies.
			sql += " and ue.status != 3"
		} else if tagInfo.Type != 1 {
			sql += " and ue.status = 0"
		} else {
			// 发件箱不展示已删除的邮件
			sql += " and ue.status != 3"
		}
	}

	if !tagInfo.Starred && tagInfo.Type == consts.EmailTypeReceive && tagInfo.Status != consts.EmailStatusDrafts {
		sql += " and type =? "
		sqlParams = append(sqlParams, tagInfo.Type)
	} else if !tagInfo.Starred && tagInfo.Type == consts.EmailTypeSend && tagInfo.Status != consts.EmailStatusDrafts {
		sql += " and (type =? or ue.group_id=? or ue.status=1)"
		sqlParams = append(sqlParams, tagInfo.Type, models.Sent)
	}

	if !tagInfo.Starred && tagInfo.GroupId != -1 {
		if tagInfo.GroupId > 0 {
			sql += " and ue.group_id=? "
			sqlParams = append(sqlParams, tagInfo.GroupId)
		} else if tagInfo.GroupId == 0 && tagInfo.Status == -1 {
			sql += " and (ue.group_id=? or ue.group_id=0)"
			sqlParams = append(sqlParams, models.INBOX)
		}
	} else {
		sql += " and (ue.group_id=0 or ue.group_id =?) "
		sqlParams = append(sqlParams, models.INBOX)
	}

	if keyword != "" {
		sql += " and (subject like ? or text like ? )"
		sqlParams = append(sqlParams, "%"+keyword+"%", "%"+keyword+"%")
	}

	if predicate != "" {
		sql += " AND (" + predicate + ")"
		sqlParams = append(sqlParams, addressArgs...)
	}

	return sql, sqlParams
}

func LoadMailboxEmails(rows []*response.UserEmailUIDData) ([]*response.EmailResponseData, error) {
	result := make([]*response.EmailResponseData, 0, len(rows))
	for start := 0; start < len(rows); start += 500 {
		batch := rows[start:min(start+500, len(rows))]
		ids := make([]int, 0, len(batch))
		for _, row := range batch {
			ids = append(ids, row.EmailID)
		}
		var messages []models.Email
		if err := db.Instance.In("id", ids).Find(&messages); err != nil {
			return nil, err
		}
		byID := make(map[int]models.Email, len(messages))
		for _, message := range messages {
			byID[message.Id] = message
		}
		for _, row := range batch {
			if message, ok := byID[row.EmailID]; ok {
				result = append(result, &response.EmailResponseData{Email: message, IsRead: row.IsRead, IsStarred: row.IsStarred, MailboxStatus: row.Status, IMAPDeleted: row.IMAPDeleted, SerialNumber: row.SerialNumber, UeId: row.ID, LinkID: row.LinkID})
			}
		}
	}
	return result, nil
}
