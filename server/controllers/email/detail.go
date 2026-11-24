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
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/token"
	"github.com/Jinnrry/pmail/services/translation"
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

	markRead := ctx.TokenScope != token.Read && ctx.TokenScope != token.Send
	email, err := detail.WebDetail(ctx, retData.ID, markRead)
	if err != nil {
		log.WithContext(ctx).Warnf("Read mail detail: %v", err)
		response.Localized(ctx, response.ServerError, "mail_not_found").FPrint(w)
		return
	}

	// Rewrite inline URLs only for the HTTP representation; MIME forwarding
	// and POP3 still need the original cid references.
	email.Html.String = inlineAttachmentURLs(retData.ID, parsemail.SanitizeHTML(email.Html.String), email.Attachments, email.AttachmentInfo)
	email.Error.String = i18n.Delivery(ctx, "delivery_detail", email.Error.String)
	if email.Text.String == "" && email.Html.String != "" {
		email.Text.String = parsemail.HTMLText(email.Html.String)
	}
	view, err := enrichedDetail(&email.Email, email.IsRead == 1, email.IsStarred, email.MailboxStatus)
	if err != nil {
		response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
		return
	}
	analysisJSON, err := json.Marshal(inbound.ReadDetail(email.InboundAnalysis, email.InboundSpam))
	var analysisFields map[string]json.RawMessage
	if err != nil || json.Unmarshal(analysisJSON, &analysisFields) != nil {
		response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
		return
	}
	for key, value := range analysisFields {
		view[key] = value
	}
	if matches, err := mailaddress.ForMessage(ctx.UserID, retData.ID); err == nil {
		view["mailbox_addresses"], _ = json.Marshal(matches)
	}
	availability := translation.Available(req.Context(), ctx.UserID, retData.ID, ctx.Lang, email.Email)
	for key, value := range map[string]any{
		"translation_available": availability.TranslationAvailable,
		"needs_translation":     availability.NeedsTranslation,
		"translation_engines":   availability.TranslationEngines,
		"ai_available":          availability.AIAvailable,
	} {
		view[key], _ = json.Marshal(value)
	}
	response.NewSuccessResponse(view).FPrint(w)
}

func inlineAttachmentURLs(emailID int, html, attachmentsJSON string, metadata ...[]models.MailAttachmentInfo) string {
	if html == "" {
		return html
	}
	var ids []string
	if len(metadata) > 0 && metadata[0] != nil {
		ids = make([]string, len(metadata[0]))
		for i, part := range metadata[0] {
			ids[i] = part.ContentID
		}
	} else {
		var attachments []struct{ ContentID string }
		if json.Unmarshal([]byte(attachmentsJSON), &attachments) != nil {
			return html
		}
		ids = make([]string, len(attachments))
		for i, a := range attachments {
			ids[i] = a.ContentID
		}
	}
	indices := make([]int, 0, len(ids))
	for i, cid := range ids {
		if cid != "" {
			indices = append(indices, i)
		}
	}
	// Try longer IDs first; keep the original zero-based index.
	sort.SliceStable(indices, func(i, j int) bool { return len(ids[indices[i]]) > len(ids[indices[j]]) })
	pairs := make([]string, 0, 2*len(indices))
	for _, index := range indices {
		pairs = append(pairs, "cid:"+ids[index], fmt.Sprintf("/api/attachments/%d.%d", emailID, index))
	}
	return strings.NewReplacer(pairs...).Replace(html)
}
