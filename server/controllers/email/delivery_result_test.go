package email

import (
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestDeliverySummaryDoesNotTreatUnreportedRecipientsAsAccepted(t *testing.T) {
	failure := errors.New("transport unavailable")
	recipients := []*parsemail.User{{EmailAddress: "first@example.test"}, {EmailAddress: "second@example.test"}}
	for _, tc := range []struct {
		name                  string
		results               map[string]error
		overall               error
		status, first, second string
	}{
		{"whole transport failure", nil, failure, "failed", "failed", "failed"},
		{"no results", nil, nil, "failed", "failed", "failed"},
		{"known delivered plus missing", map[string]error{"first@example.test": nil}, failure, "sent", "delivered", "failed"},
		{"queue and reject", map[string]error{"first@example.test": queue.ErrQueued, "second@example.test": failure}, failure, "queued", "queued", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeDelivery(&context.Context{Lang: "en"}, recipients, tc.results, tc.overall)
			if got.Status != tc.status || len(got.Recipients) != 2 || got.Recipients[0].Status != tc.first || got.Recipients[1].Status != tc.second || got.firstFailure == nil || got.Recipients[1].Error == "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}
