package imap_server

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	mtp "github.com/emersion/go-message/textproto"
	"github.com/spf13/cast"
)

// userToAddress 将 parsemail.User 转换为 imap.Address
func userToAddress(u *parsemail.User) imap.Address {
	if u == nil {
		return imap.Address{}
	}
	// 解析邮箱地址的 local 和 domain 部分
	mailbox := u.EmailAddress
	host := ""
	if at := strings.LastIndex(u.EmailAddress, "@"); at >= 0 {
		mailbox = u.EmailAddress[:at]
		host = u.EmailAddress[at+1:]
	}
	return imap.Address{
		Name:    u.Name,
		Mailbox: mailbox,
		Host:    host,
	}
}

// usersToAddresses 将用户列表转换为地址列表
func usersToAddresses(users []*parsemail.User) []imap.Address {
	if len(users) == 0 {
		return nil
	}
	addrs := make([]imap.Address, 0, len(users))
	for _, u := range users {
		if u != nil && u.EmailAddress != "" {
			addrs = append(addrs, userToAddress(u))
		}
	}
	if len(addrs) == 0 {
		return nil
	}
	return addrs
}

// buildEnvelope 构建 IMAP ENVELOPE 结构
func buildEnvelope(email *response.EmailResponseData, traEmail *parsemail.Email) *imap.Envelope {
	// From 地址
	var from []imap.Address
	if email.FromAddress != "" {
		mailbox := email.FromAddress
		host := ""
		if at := strings.LastIndex(email.FromAddress, "@"); at >= 0 {
			mailbox = email.FromAddress[:at]
			host = email.FromAddress[at+1:]
		}
		from = []imap.Address{{
			Name:    email.FromName,
			Mailbox: mailbox,
			Host:    host,
		}}
	}

	// Sender (如果没有单独的 sender，使用 from)
	var sender []imap.Address
	if traEmail.Sender != nil && traEmail.Sender.EmailAddress != "" {
		sender = []imap.Address{userToAddress(traEmail.Sender)}
	} else {
		sender = from
	}

	// Reply-To
	var replyTo []imap.Address
	if len(traEmail.ReplyTo) > 0 {
		replyTo = usersToAddresses(traEmail.ReplyTo)
	} else {
		replyTo = from
	}

	// Message-ID: use the persisted value so IMAP clients see a stable ID on every sync.
	var messageID string
	if email.MsgID != "" {
		messageID = fmt.Sprintf("<%s>", email.MsgID)
	} else {
		messageID = fmt.Sprintf("<%d@%s>", email.Id, domain.DefaultName())
	}

	return &imap.Envelope{
		Date:      email.CreateTime,
		Subject:   email.Subject,
		From:      from,
		Sender:    sender,
		ReplyTo:   replyTo,
		To:        usersToAddresses(traEmail.To),
		Cc:        usersToAddresses(traEmail.Cc),
		Bcc:       usersToAddresses(traEmail.Bcc),
		MessageID: messageID,
		// InReplyTo 和 References 暂不支持
	}
}

func (s *serverSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	state, err := mailboxSnapshot(s.ctx, s.currentMailbox)
	if err != nil {
		return protocolNO(err)
	}
	matched := present(s.selectSet(state.items, numSet))
	emails, err := list.LoadMailboxEmails(matched)
	if err != nil {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not read mailbox messages"}
	}
	return write(s.ctx, w, emails, options, s.readOnly)
}

// 纯文本 text/plain
func bsTextPlain(size uint32, numLines int64) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:     "text",
		Subtype:  "plain",
		Params:   map[string]string{"charset": "utf-8"},
		Encoding: "base64",
		Size:     size, // 按字节数
		Text:     &imap.BodyStructureText{NumLines: numLines},
		Extended: &imap.BodyStructureSinglePartExt{},
	}
}

// HTML text/html
func bsTextHTML(size uint32, numLines int64) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type:     "text",
		Subtype:  "html",
		Params:   map[string]string{"charset": "utf-8"},
		Encoding: "base64",
		Size:     size,
		Text:     &imap.BodyStructureText{NumLines: numLines},
		Extended: &imap.BodyStructureSinglePartExt{},
	}
}

// 通用附件（传入 MIME，如 "application/pdf"）
func bsAttachment(filename, mime string, size uint32, encoding string) *imap.BodyStructureSinglePart {
	mt, st := "application", "octet-stream"
	if slash := strings.IndexByte(mime, '/'); slash > 0 {
		mt, st = mime[:slash], mime[slash+1:]
	}
	return &imap.BodyStructureSinglePart{
		Type:     mt,
		Subtype:  st,
		Params:   map[string]string{"name": filename}, // 备用名
		ID:       "",                                  // 可填 Content-ID
		Encoding: encoding,                            // 常见 "base64"
		Size:     size,
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value:  "attachment",
				Params: map[string]string{"filename": filename}, // 客户端优先用这里
			},
		},
	}
}

// multipart/alternative：text + html
// 注意：BodyStructureMultiPart 必须至少有一个子元素，否则 go-imap 会 panic
func bsAlternative(text, html *imap.BodyStructureSinglePart) *imap.BodyStructureMultiPart {
	var children []imap.BodyStructure

	if text != nil {
		children = append(children, text)
	}
	if html != nil {
		children = append(children, html)
	}

	// 如果没有任何内容，创建一个空的 text/plain 占位符
	// 这可以避免 go-imap 的 panic: "imap.BodyStructureMultiPart must have at least one child"
	if len(children) == 0 {
		children = append(children, bsTextPlain(0, 0))
	}

	return &imap.BodyStructureMultiPart{
		Subtype:  "alternative",
		Children: children,
		Extended: &imap.BodyStructureMultiPartExt{},
	}
}

// multipart/mixed：{ alternative(text+html), attachments... }
func bsMixedWithAttachments(alt *imap.BodyStructureMultiPart, extend bool, atts ...imap.BodyStructure) *imap.BodyStructureMultiPart {
	children := []imap.BodyStructure{alt}
	children = append(children, atts...)
	var ext *imap.BodyStructureMultiPartExt
	if extend {
		ext = &imap.BodyStructureMultiPartExt{}
	}
	return &imap.BodyStructureMultiPart{
		Subtype:  "mixed",
		Children: children,
		Extended: ext,
	}
}

func write(ctx *context.Context, w *imapserver.FetchWriter, emailList []*response.EmailResponseData, options *imap.FetchOptions, readOnly ...bool) error {
	for _, email := range emailList {
		traEmail := parsemail.NewEmailFromModel(email.Email)
		var emailContent []byte
		if options.RFC822Size || len(options.BodySection) > 0 {
			var err error
			emailContent, err = traEmail.BuildBytes(ctx, false)
			if err != nil {
				return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not encode mailbox message"}
			}
		}
		writer := w.CreateMessage(cast.ToUint32(email.SerialNumber))

		if options.UID {
			writer.WriteUID(imap.UID(email.UeId))
		}
		if options.Envelope {
			env := buildEnvelope(email, traEmail)
			writer.WriteEnvelope(env)
		}
		if options.BodyStructure != nil {
			var html, text *imap.BodyStructureSinglePart
			if len(traEmail.HTML) > 0 {
				html = bsTextHTML(uint32(len(traEmail.HTML)), int64(bytes.Count(traEmail.HTML, []byte("\n"))+1))
			}

			if len(traEmail.Text) > 0 {
				text = bsTextPlain(uint32(len(traEmail.Text)), int64(bytes.Count(traEmail.Text, []byte("\n"))+1))
			}

			alt := bsAlternative(text, html)

			var attrs []imap.BodyStructure
			for _, attachment := range traEmail.Attachments {
				if attachment != nil {
					attrs = append(attrs, bsAttachment(attachment.Filename, attachment.ContentType, uint32(len(attachment.Content)), "base64"))
				}
			}
			bs := bsMixedWithAttachments(alt, options.BodyStructure.Extended, attrs...) // 最终的 BodyStructure（接口值）

			writer.WriteBodyStructure(bs)
		}
		if options.RFC822Size {
			writer.WriteRFC822Size(cast.ToInt64(len(emailContent)))
		}
		for _, section := range options.BodySection {
			if !section.Peek && email.IsRead == 0 && (len(readOnly) == 0 || !readOnly[0]) {
				if email.LinkID > 0 {
					if _, err := db.Instance.ID(email.LinkID).Where("user_id=?", ctx.UserID).Cols("is_read").Update(&models.UserEmail{IsRead: 1}); err == nil {
						email.IsRead = 1
					}
				}
				break
			}
		}
		if options.Flags {
			writer.WriteFlags(mailFlags(email.IsRead == 1, email.IsStarred, email.MailboxStatus, email.IMAPDeleted))
		}
		if options.InternalDate {
			writer.WriteInternalDate(email.CreateTime)
		}
		for _, section := range options.BodySection {

			if len(section.HeaderFields) > 0 || len(section.HeaderFieldsNot) > 0 || section.Specifier == imap.PartSpecifierHeader {
				hdr, err := mtp.ReadHeader(bufio.NewReader(bytes.NewReader(emailContent)))
				if err != nil {
					writer.Close()
					return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not encode message headers"}
				}
				wanted := func(fields []string, key string) bool {
					for _, name := range fields {
						if strings.EqualFold(name, key) {
							return true
						}
					}
					return false
				}
				fields := hdr.Fields()
				for fields.Next() {
					if (len(section.HeaderFields) > 0 && !wanted(section.HeaderFields, fields.Key())) || wanted(section.HeaderFieldsNot, fields.Key()) {
						fields.Del()
					}
				}
				var b bytes.Buffer
				if err := mtp.WriteHeader(&b, hdr); err != nil {
					writer.Close()
					return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not encode message headers"}
				}
				bodyWriter := writer.WriteBodySection(section, int64(b.Len()))
				_, _ = bodyWriter.Write(b.Bytes())
				_ = bodyWriter.Close()
			} else if section.Specifier == imap.PartSpecifierNone || section.Specifier == imap.PartSpecifierText {
				if len(section.Part) >= 1 {
					// 获取指定 part 的内容
					partContent := traEmail.BuildPart(ctx, section.Part)
					if partContent != nil {
						bodyWriter := writer.WriteBodySection(section, cast.ToInt64(len(partContent)))
						bodyWriter.Write(partContent)
						bodyWriter.Close()
					} else {
						// Part 不存在，返回空
						bodyWriter := writer.WriteBodySection(section, 0)
						bodyWriter.Close()
					}
				} else {
					bodyWriter := writer.WriteBodySection(section, cast.ToInt64(len(emailContent)))
					bodyWriter.Write(emailContent)
					bodyWriter.Close()
				}
			}
		}
		writer.Close()
	}
	return nil
}
