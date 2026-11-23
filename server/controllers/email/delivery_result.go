package email

import (
	"errors"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
)

type recipientResult struct {
	Address string `json:"address"`
	Status  string `json:"status"`
	Error   string `json:"error"`
}

type deliverySummary struct {
	Status       string            `json:"status"`
	Recipients   []recipientResult `json:"recipients"`
	firstFailure error
}

var errMissingDeliveryResult = errors.New("mail delivery returned no recipient result")

func summarizeDelivery(ctx *context.Context, to []*parsemail.User, results map[string]error, overall error) deliverySummary {
	out := deliverySummary{Status: "failed", Recipients: make([]recipientResult, 0, len(to))}
	for _, recipient := range to {
		err, reported := results[recipient.EmailAddress]
		if !reported {
			// A preflight failure (e.g. local transport unavailable) can return no
			// recipient results. In particular, never delete a draft in that case.
			err = overall
			if err == nil {
				err = errMissingDeliveryResult
			}
		}
		item := recipientResult{Address: recipient.EmailAddress}
		switch {
		case reported && errors.Is(err, queue.ErrQueued):
			item.Status = "queued"
			out.Status = "queued"
		case err != nil:
			item.Status = "failed"
			item.Error = i18n.Delivery(ctx, "delivery_detail", err.Error())
			if out.firstFailure == nil {
				out.firstFailure = err
			}
		default:
			item.Status = "delivered"
			if out.Status != "queued" {
				out.Status = "sent"
			}
		}
		out.Recipients = append(out.Recipients, item)
	}
	if out.firstFailure == nil {
		out.firstFailure = overall
	}
	return out
}
