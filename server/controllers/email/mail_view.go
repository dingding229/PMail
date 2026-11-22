package email

import (
	"encoding/json"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailstore"
)

type mailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type normalizedMail struct {
	ToList          []mailAddress               `json:"to_list"`
	CcList          []mailAddress               `json:"cc_list"`
	BccList         []mailAddress               `json:"bcc_list"`
	ReplyToList     []mailAddress               `json:"reply_to_list"`
	AttachmentsList []models.MailAttachmentInfo `json:"attachments_list"`
}

func normalizeMail(row models.Email) normalizedMail {
	addresses := func(value string) []mailAddress {
		var values []parsemail.User
		_ = json.Unmarshal([]byte(value), &values)
		out := make([]mailAddress, 0, len(values))
		for _, item := range values {
			if item.EmailAddress != "" {
				out = append(out, mailAddress{item.Name, item.EmailAddress})
			}
		}
		return out
	}
	files := row.AttachmentInfo
	if files == nil {
		var stored []parsemail.Attachment
		_ = json.Unmarshal([]byte(row.Attachments), &stored)
		files = mailstore.Metadata(stored)
	}
	return normalizedMail{addresses(row.To), addresses(row.Cc), addresses(row.Bcc), addresses(row.ReplyTo), files}
}

type mailAuthenticationView struct {
	IsDraft     bool                    `json:"is_draft"`
	Dangerous   bool                    `json:"dangerous"`
	AuthStatus  string                  `json:"auth_status"`
	AuthResults *models.MailAuthResults `json:"auth_results"`
}

func mailAuthentication(row models.Email, mailboxStatus int8) mailAuthenticationView {
	evidence := row.TransportAuth()
	draft := row.Status == consts.EmailStatusDrafts || mailboxStatus == consts.EmailStatusDrafts
	switch {
	case draft || row.Type == consts.EmailTypeSend || mailboxStatus == consts.EmailStatusSent:
		return mailAuthenticationView{IsDraft: draft, AuthStatus: "not_applicable", AuthResults: evidence}
	case evidence != nil && !row.AuthUnchecked:
		status := evidence.Status()
		return mailAuthenticationView{AuthStatus: status, Dangerous: status == "fail", AuthResults: evidence}
	case row.AuthUnchecked:
		return mailAuthenticationView{AuthStatus: "none"}
	case row.SPFCheck == 1 || row.DKIMCheck == 1:
		return mailAuthenticationView{AuthStatus: "pass"}
	case row.SPFCheck == 0 && row.DKIMCheck == 0:
		return mailAuthenticationView{Dangerous: true, AuthStatus: "fail"}
	default:
		return mailAuthenticationView{AuthStatus: "none"}
	}
}

// The legacy model has a custom JSON marshaler. Merge the extra HTTP fields
// without changing the original field encodings or rounding numeric IDs.
func enrichedDetail(row *models.Email, read, starred bool, mailboxStatus int8) (map[string]json.RawMessage, error) {
	base, err := row.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(base, &view); err != nil {
		return nil, err
	}
	format, hasCode := parsemail.CodeMetadata(row.Text.String, row.Html.String, row.TextFormat)
	extra, err := json.Marshal(struct {
		normalizedMail
		mailAuthenticationView
		IsRead        bool   `json:"is_read"`
		IsStarred     bool   `json:"is_starred"`
		TextFormat    string `json:"text_format"`
		HasCode       bool   `json:"has_code"`
		MailboxStatus int8   `json:"mailbox_status"`
	}{normalizeMail(*row), mailAuthentication(*row, mailboxStatus), read, starred, format, hasCode, mailboxStatus})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(extra, &view); err != nil {
		return nil, err
	}
	return view, nil
}
