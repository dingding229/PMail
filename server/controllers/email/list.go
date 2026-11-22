package email

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/isotime"
	"github.com/spf13/cast"
)

type emailListResponse struct {
	CurrentPage int         `json:"current_page"`
	TotalPage   int         `json:"total_page"`
	List        []*emilItem `json:"list"`
}

type emilItem struct {
	normalizedMail
	models.EmailISOTimes
	mailAuthenticationView
	DatetimeISO *time.Time `json:"datetime_iso"`
	IsStarred   bool       `json:"is_starred"`
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Desc        string     `json:"desc"`
	Datetime    string     `json:"datetime"`
	IsRead      bool       `json:"is_read"`
	Sender      User       `json:"sender"`
	To          []User     `json:"to"`
	Error       string     `json:"error"`
}

type User struct {
	Name         string `json:"Name"`
	EmailAddress string `json:"EmailAddress"`
}

type emailRequest struct {
	Keyword     string `json:"keyword"`
	Tag         string `json:"tag"`
	CurrentPage int    `json:"current_page"`
	PageSize    int    `json:"page_size"`
}

func EmailList(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var lst []*emilItem
	var retData emailRequest
	if !readMailJSON(ctx, w, req, &retData, true) {
		return
	}

	if retData.PageSize == 0 {
		retData.PageSize = 15
	}
	if retData.PageSize < 1 || retData.PageSize > 100 || retData.CurrentPage < 0 || retData.CurrentPage > 1000000 {
		response.Localized(ctx, response.ParamsError, "invalid_pagination").FPrint(w)
		return
	}
	offset := max(0, retData.CurrentPage-1) * retData.PageSize

	var tagInfo dto.SearchTag = dto.SearchTag{
		Type:    -1,
		Status:  -1,
		GroupId: -1,
	}
	_ = json.Unmarshal([]byte(retData.Tag), &tagInfo)

	emailList, total := list.GetEmailList(ctx, tagInfo, retData.Keyword, false, offset, retData.PageSize)

	for _, email := range emailList {
		var sender User
		_ = json.Unmarshal([]byte(email.Sender), &sender)

		if sender.EmailAddress == "" {
			sender.EmailAddress = email.FromAddress
			sender.Name = email.FromName
		}

		var tos []User
		_ = json.Unmarshal([]byte(email.To), &tos)

		lst = append(lst, &emilItem{
			normalizedMail:         normalizeMail(email.Email),
			EmailISOTimes:          email.Email.ISOTimes(),
			mailAuthenticationView: mailAuthentication(email.Email, email.MailboxStatus),
			DatetimeISO:            isotime.UTC(email.SendDate),
			IsStarred:              email.IsStarred,
			ID:                     email.Id,
			Title:                  email.Subject,
			Desc:                   email.Text.String,
			Datetime:               email.SendDate.Format("2006-01-02 15:04:05"),
			IsRead:                 email.IsRead == 1,
			Sender:                 sender,
			To:                     tos,
			Error:                  i18n.Delivery(ctx, "delivery_detail", email.Error.String),
		})
	}

	ret := emailListResponse{
		CurrentPage: retData.CurrentPage,
		TotalPage:   cast.ToInt(math.Ceil(cast.ToFloat64(total) / cast.ToFloat64(retData.PageSize))),
		List:        lst,
	}
	response.NewSuccessResponse(ret).FPrint(w)
}
