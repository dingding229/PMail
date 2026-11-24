// Package backupcatalog defines the persistent tables shared by offline migration and disaster recovery.
package backupcatalog

import "slices"

// Required tables are present in the oldest supported source. The rest
// may not exist yet; do not confuse an absent source with a target error.
var Required = []string{"user", "email", "group", "rule", "user_email", "version"}
var Optional = []string{
	"domain", "mailbox_address", "address_settings", "api_token",
	"totp_config", "recovery_code", "app_password",
	"user_email_address", "user_email_address_progress", "imap_mailbox", "imap_mailbox_uid",
	"forward_setting", "outbound_payload", "outbound_queue", "bounce_notice", "inbound_settings", "trash_settings",
	"spam_preference", "spam_rule", "spam_training", "spam_totals", "spam_token",
	"telegram_config", "telegram_binding", "telegram_code", "telegram_preference", "telegram_push",
	"compose_image", "draft_inline_image", "signature", "translation_settings", "user_ai_config",
	"translation_preference", "translation_usage", "email_translation", "email_ai_result", "mail_archive_settings",
}
var Tables = slices.Concat(Required, Optional)

// Excluded tables are deliberately recreated, not exported.
var Excluded = []string{"sessions", "mail_attachment_info", "mail_attachment_chunk", "registration_guard"}

func KeyColumn(table string) string {
	switch table {
	case "totp_config", "forward_setting", "spam_preference", "spam_totals", "telegram_binding", "telegram_code", "telegram_preference", "user_ai_config", "translation_preference", "translation_usage":
		return "user_id"
	case "recovery_code", "user_email_address", "imap_mailbox_uid", "spam_training", "spam_token", "compose_image", "draft_inline_image", "email_translation", "email_ai_result":
		return ""
	}
	return "id"
}

func HasIDSequence(table string) bool {
	if KeyColumn(table) != "id" {
		return false
	}
	switch table {
	case "address_settings", "inbound_settings", "trash_settings", "telegram_config", "translation_settings", "user_email_address_progress", "mail_archive_settings":
		return false
	}
	return true
}

func Binary(table, column string) bool {
	return column == "wire_data" || column == "raw_data" || (table == "compose_image" && column == "content")
}
