package email

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

type emailDetailRequest struct {
	ID int `json:"id"`
}

func EmailDetail(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var retData emailDetailRequest
	if !readMailJSON(ctx, w, req, &retData, false) {
		return
	}

	if retData.ID <= 0 {
		response.Localized(ctx, response.ParamsError, "invalid_id").FPrint(w)
		return
	}

	email, err := detail.GetEmailDetail(ctx, retData.ID, true)
	if err != nil {
		log.WithContext(ctx).Warnf("Read mail detail: %v", err)
		response.Localized(ctx, response.ServerError, "mail_not_found").FPrint(w)
		return
	}

	// Rewrite inline URLs only for the HTTP representation; MIME forwarding
	// and POP3 still need the original cid references.
	email.Html.String = inlineAttachmentURLs(retData.ID, email.Html.String, email.Attachments)
	email.Error.String = i18n.Delivery(ctx, "delivery_detail", email.Error.String)
	response.NewSuccessResponse(email).FPrint(w)
}

func inlineAttachmentURLs(emailID int, html, attachmentsJSON string) string {
	if html == "" || attachmentsJSON == "" {
		return html
	}
	var attachments []parsemail.Attachment
	if err := json.Unmarshal([]byte(attachmentsJSON), &attachments); err != nil {
		return html
	}
	// Try longer IDs first: cid:example must not consume the prefix of
	// cid:example-extra. Keep the attachment's original zero-based index.
	indices := make([]int, 0, len(attachments))
	for i, attachment := range attachments {
		if attachment.ContentID != "" {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return len(attachments[indices[i]].ContentID) > len(attachments[indices[j]].ContentID)
	})
	pairs := make([]string, 0, 2*len(indices))
	for _, index := range indices {
		pairs = append(pairs, "cid:"+attachments[index].ContentID, fmt.Sprintf("/api/attachments/%d.%d", emailID, index))
	}
	return strings.NewReplacer(pairs...).Replace(html)
}
