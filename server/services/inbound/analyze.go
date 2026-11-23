package inbound

import (
	"context"
	"net"
	"slices"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

const Budget = 1500 * time.Millisecond
const optionalBudget = 900 * time.Millisecond

var analysisSlots = make(chan struct{}, 8)

func Analyze(ctx context.Context, e *parsemail.Email, env Envelope, auth *models.MailAuthResults) *Analysis {
	a, _ := AnalyzeDelivery(ctx, e, env, auth, nil)
	return a
}

// AnalyzeDelivery shares the content work and one deadline among recipients.
// Its result is advisory: no analysis or setting error can reject delivery.
func AnalyzeDelivery(parent context.Context, e *parsemail.Email, env Envelope, auth *models.MailAuthResults, owners []int) (*Analysis, map[int]*Spam) {
	if e == nil {
		return nil, nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, Budget)
	defer cancel()
	if ctx.Err() != nil {
		return nil, nil
	}
	select {
	case analysisSlots <- struct{}{}:
	default:
		return nil, nil
	}
	type result struct {
		shared   *Analysis
		perOwner map[int]*Spam
	}
	pending := make(chan result, 1)
	engine := db.Instance // keep the generation pinned if the caller times out during a reload
	snapshot := *e        // the parser-owned body and attachment buffers are immutable
	go func() {
		defer func() { <-analysisSlots }()
		a, scores := analyzeDelivery(ctx, engine, &snapshot, env, auth, owners)
		pending <- result{a, scores}
	}()
	select {
	case <-ctx.Done():
		return nil, nil
	case r := <-pending:
		if ctx.Err() != nil {
			return nil, nil
		}
		return r.shared, r.perOwner
	}
}

func analyzeDelivery(ctx context.Context, engine *xorm.Engine, e *parsemail.Email, env Envelope, auth *models.MailAuthResults, owners []int) (*Analysis, map[int]*Spam) {
	settings, err := getSettings(ctx, engine)
	if err != nil {
		settings = Settings{DefaultSpamThreshold: 6}
	}
	if ctx.Err() != nil {
		return nil, nil
	}
	a := analyzeShared(ctx, e, env, auth, settings)
	if a == nil {
		return nil, nil
	}
	scores := map[int]*Spam{}
	if engine == nil {
		return a, scores
	}
	for _, uid := range owners {
		if ctx.Err() != nil {
			return nil, nil
		}
		if _, exists := scores[uid]; exists {
			continue
		}
		score, err := classify(ctx, engine, uid, e, env, a.Base, settings.DefaultSpamThreshold)
		if err == nil {
			scores[uid] = score
		}
	}
	if ctx.Err() != nil {
		return nil, nil
	}
	return a, scores
}

type dnsAnswer struct {
	zone   string
	listed bool
}

func analyzeShared(ctx context.Context, e *parsemail.Email, env Envelope, auth *models.MailAuthResults, s Settings) *Analysis {
	if ctx.Err() != nil {
		return nil
	}
	remote, cancel := context.WithTimeout(ctx, optionalBudget)
	defer cancel()
	replies := make(chan dnsAnswer, len(s.Zones))
	pending := 0
	if s.DNSBLEnabled && cacheableIP(env.SourceIP) {
		for _, zone := range s.Zones {
			if !acquireNetwork() {
				continue
			}
			pending++
			go func() {
				defer releaseNetwork()
				r := health.dns(remote, env.SourceIP, zone, net.DefaultResolver.LookupNetIP)
				replies <- dnsAnswer{zone, r.listed}
			}()
		}
	}
	a := &Analysis{Version: 1, Risk: &Risk{Level: "none", Findings: []Finding{}}, Base: baseReasons(e, auth)}
	scan := riskScan{a}
	scan.text(ctx, clippedBytes(e.Text, maxScanBody), -1, 0)
	scan.base64(ctx, clippedBytes(e.Text, maxScanBody))
	if len(e.HTML) > 0 && !e.TextFromHTML {
		body := []byte(parsemail.HTMLText(string(clippedBytes(e.HTML, maxScanBody))))
		scan.text(ctx, clippedBytes(body, maxScanBody), -1, 0)
		scan.base64(ctx, clippedBytes(body, maxScanBody))
	}
	for i, part := range e.Attachments {
		if ctx.Err() != nil {
			return nil
		}
		scan.attachment(ctx, i, part)
	}
	a.VerificationCode = recognizeCode(e)
	var zones []string
	for pending > 0 {
		select {
		case reply := <-replies:
			pending--
			if reply.listed {
				zones = append(zones, reply.zone)
			}
		case <-remote.Done():
			// Keep DNS replies that were already queued at the deadline.
			for {
				select {
				case reply := <-replies:
					if reply.listed {
						zones = append(zones, reply.zone)
					}
				default:
					goto drained
				}
			}
		drained:
			pending = 0
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	slices.Sort(zones)
	for _, zone := range zones {
		a.Base = append(a.Base, Reason{Code: "dnsbl_listed", Weight: weights["dnsbl_listed"], Detail: zone})
	}
	return a
}
