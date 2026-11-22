package email

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/isotime"
)

type emailListResponse struct {
	CurrentPage int         `json:"current_page"`
	TotalPage   int         `json:"total_page"`
	List        []*emilItem `json:"list"`
	NextCursor  *string     `json:"next_cursor,omitempty"`
}

type emilItem struct {
	normalizedMail
	inbound.ListView
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
	Keyword      string  `json:"keyword"`
	Tag          string  `json:"tag"`
	CurrentPage  int     `json:"current_page"`
	PageSize     int     `json:"page_size"`
	Cursor       *string `json:"cursor"`
	IncludeTotal *bool   `json:"include_total"`
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

	limit := retData.PageSize
	count := retData.Cursor == nil
	if retData.IncludeTotal != nil {
		count = *retData.IncludeTotal
	}
	var cursor list.MailCursor
	if retData.Cursor != nil {
		var err error
		cursor, err = list.DecodeCursor(*retData.Cursor)
		if err != nil {
			response.Localized(ctx, response.ParamsError, "invalid_pagination").FPrint(w)
			return
		}
		offset = 0
		limit++
	}
	emailList, total, err := list.WebPage(ctx, tagInfo, retData.Keyword, cursor, offset, limit, count)
	if err != nil {
		response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
		return
	}
	var next *string
	if retData.Cursor != nil {
		value := ""
		if len(emailList) > retData.PageSize {
			emailList = emailList[:retData.PageSize]
			value = list.EncodeCursor(emailList[len(emailList)-1])
		}
		next = &value
	}

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
			ListView:               inbound.ReadList(email.InboundAnalysis, email.InboundSpam),
			EmailISOTimes:          email.Email.ISOTimes(),
			mailAuthenticationView: mailAuthentication(email.Email, email.MailboxStatus),
			DatetimeISO:            isotime.UTC(email.SendDate),
			IsStarred:              email.IsStarred,
			ID:                     email.Id,
			Title:                  email.Subject,
			Desc:                   email.ListSnippet,
			Datetime:               email.SendDate.Format("2006-01-02 15:04:05"),
			IsRead:                 email.IsRead == 1,
			Sender:                 sender,
			To:                     tos,
			Error:                  i18n.Delivery(ctx, "delivery_detail", email.Error.String),
		})
	}

	ret := emailListResponse{
		CurrentPage: retData.CurrentPage,
		TotalPage:   int((total + int64(retData.PageSize) - 1) / int64(retData.PageSize)),
		List:        lst,
		NextCursor:  next,
	}
	response.NewSuccessResponse(ret).FPrint(w)
}
