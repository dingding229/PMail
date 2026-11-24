package group

import (
	errors2 "errors"
	"slices"
	"strings"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

type GroupItem struct {
	Unread   int          `json:"unread"`
	Total    int          `json:"total"`
	Id       int          `json:"id"`
	Label    string       `json:"label"`
	Tag      string       `json:"tag"`
	Children []*GroupItem `json:"children"`
}

const maxGroupDepth = 16

var ErrInvalidParent = errors2.New("invalid parent or group hierarchy")

func parentPath(owner, parent int, excluded int) (string, error) {
	seen := map[int]bool{}
	var parts []string
	for parent != 0 {
		if parent < 0 || parent == excluded || seen[parent] || len(parts) >= maxGroupDepth-1 {
			return "", ErrInvalidParent
		}
		seen[parent] = true
		var g models.Group
		ok, err := db.Instance.ID(parent).Where("user_id=?", owner).Get(&g)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrInvalidParent
		}
		parts = append(parts, g.Name)
		parent = g.ParentId
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/"), nil
}

func GroupPath(ctx *context.Context, row *models.Group) (string, error) {
	if row.UserId != ctx.UserID {
		return "", ErrInvalidParent
	}
	path, err := parentPath(ctx.UserID, row.ParentId, row.ID)
	if err != nil {
		return "", err
	}
	if path == "" {
		return row.Name, nil
	}
	return path + "/" + row.Name, nil
}

func ValidMoveTarget(ctx *context.Context, id int) bool {
	if _, ok := models.GroupCodeToName[id]; ok {
		return true
	}
	if id <= 0 {
		return false
	}
	ok, err := db.Instance.ID(id).Where("user_id=?", ctx.UserID).Exist(&models.Group{})
	return err == nil && ok
}

func CreateGroup(ctx *context.Context, name string, parentId int) (*models.Group, error) {
	prefix, pathErr := parentPath(ctx.UserID, parentId, 0)
	if pathErr != nil {
		return nil, pathErr
	}
	// 先查询是否存在
	var group models.Group
	found, err := db.Instance.Where("name = ? and user_id = ?", name, ctx.UserID).Get(&group)
	if err != nil {
		return nil, err
	}
	if found {
		return &group, nil
	}
	group.Name = name
	group.ParentId = parentId
	group.UserId = ctx.UserID
	group.FullPath = name
	if prefix != "" {
		group.FullPath = prefix + "/" + name
	}

	_, err = db.Instance.Insert(&group)
	return &group, err
}

func Rename(ctx *context.Context, oldName, newName string) error {
	oldGroupInfo, err := GetGroupByName(ctx, oldName)
	if err != nil {
		return err
	}
	if oldGroupInfo == nil || oldGroupInfo.ID == 0 {
		return errors2.New("group not found")
	}
	oldGroupInfo.Name = newName
	prefix, err := parentPath(ctx.UserID, oldGroupInfo.ParentId, oldGroupInfo.ID)
	if err != nil {
		return err
	}
	oldGroupInfo.FullPath = newName
	if prefix != "" {
		oldGroupInfo.FullPath = prefix + "/" + newName
	}
	_, err = db.Instance.ID(oldGroupInfo.ID).Update(oldGroupInfo)
	return err
}

func GetGroupByName(ctx *context.Context, name string) (*models.Group, error) {
	var group models.Group
	db.Instance.Table("group").Where("name = ? and user_id = ?", name, ctx.UserID).Get(&group)

	return &group, nil
}

func DelGroup(ctx *context.Context, groupId int) (bool, error) {
	trans := db.Instance.NewSession()
	defer trans.Close()
	if err := trans.Begin(); err != nil {
		return false, errors.Wrap(err)
	}
	defer trans.Rollback()

	var root models.Group
	found, err := trans.Where("id=? and user_id=?", groupId, ctx.UserID).Get(&root)
	if err != nil {
		return false, errors.Wrap(err)
	}
	if !found {
		return false, nil
	}

	// Only collect descendants owned by this user, in the same transaction.
	allGroupIds := []int{groupId}
	seen := map[int]bool{groupId: true}
	for pending := []int{groupId}; len(pending) > 0; {
		var children []models.Group
		if err := trans.Where("user_id=?", ctx.UserID).In("parent_id", pending).Find(&children); err != nil {
			return false, errors.Wrap(err)
		}
		pending = nil
		for _, child := range children {
			if !seen[child.ID] {
				seen[child.ID] = true
				allGroupIds = append(allGroupIds, child.ID)
				pending = append(pending, child.ID)
			}
		}
	}

	num, err := trans.Where("user_id=?", ctx.UserID).In("id", allGroupIds).Delete(&models.Group{})
	if err != nil {
		return false, errors.Wrap(err)
	}
	_, err = trans.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).
		In("group_id", allGroupIds).Update(map[string]interface{}{"group_id": 0})
	if err != nil {
		return false, errors.Wrap(err)
	}
	if err := trans.Commit(); err != nil {
		return false, errors.Wrap(err)
	}
	if num > 0 {
		mailevents.Publish(ctx.UserID)
	}
	return num > 0, nil
}

// GetGroupInfoList 获取全部的分组
func GetGroupInfoList(ctx *context.Context) []*GroupItem {
	var rows []models.Group
	if err := db.Instance.Where("user_id=?", ctx.UserID).Asc("id").Find(&rows); err != nil {
		log.WithContext(ctx).Errorf("read groups: %v", err)
		return nil
	}
	byParent := map[int][]models.Group{}
	for _, r := range rows {
		byParent[r.ParentId] = append(byParent[r.ParentId], r)
	}
	return buildChildren(byParent, map[int]bool{}, 0, 0)
}

// MoveMailToGroup moves the owner's messages into a custom folder. A direct
// move out of Trash must clear the expiry and the old IMAP UID projection.
func MoveMailToGroup(ctx *context.Context, mailIDs []int, groupID int) bool {
	if groupID <= 0 || len(mailIDs) == 0 {
		return false
	}
	var changed bool
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		changed = false
		if ctx.Context != nil {
			tx.Context(ctx.Context)
		}
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		q := tx.ID(groupID).Where("user_id=?", ctx.UserID).Cols("id")
		if tx.Engine().DriverName() != "sqlite" {
			q.ForUpdate()
		}
		if found, err := q.Get(&models.Group{}); err != nil || !found {
			return err
		}
		for batch := range slices.Chunk(mailIDs, del_email.BatchSize) {
			var trashed []int
			if err := tx.Table(&models.UserEmail{}).Cols("id").Where("user_id=? AND status=?", ctx.UserID, consts.EmailStatusDel).In("email_id", batch).Find(&trashed); err != nil {
				return err
			}
			n, err := tx.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).In("email_id", batch).Update(map[string]any{"group_id": groupID})
			if err != nil {
				return err
			}
			changed = changed || n > 0
			if err := del_email.ClearTrashMetadata(tx, trashed); err != nil {
				return err
			}
			// Custom folders list status 0 links, so make the moved copy visible.
			for links := range slices.Chunk(trashed, del_email.BatchSize) {
				if _, err := tx.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).In("id", links).Update(map[string]any{"status": consts.EmailStatusWait}); err != nil {
					return err
				}
				changed = true
			}
		}
		return nil
	})
	if err != nil {
		log.WithContext(ctx).Errorf("move mail to folder: %v", err)
		return false
	}
	return changed
}

func buildChildren(byParent map[int][]models.Group, seen map[int]bool, parent, depth int) []*GroupItem {
	if depth >= maxGroupDepth {
		return nil
	}
	var out []*GroupItem
	for _, g := range byParent[parent] {
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		out = append(out, &GroupItem{Id: g.ID, Label: g.Name, Tag: dto.SearchTag{GroupId: g.ID, Status: -1, Type: -1}.ToString(), Children: buildChildren(byParent, seen, g.ID, depth+1)})
	}
	return out
}

func GetGroupList(ctx *context.Context) []*models.Group {
	var ret []*models.Group
	db.Instance.Table("group").Where("user_id=?", ctx.UserID).Find(&ret)
	return ret
}

func IsDefaultBox(box string) bool {
	return array.InArray(box, []string{"INBOX", "Sent Messages", "Drafts", "Deleted Messages", "Junk"})
}

func Move2DefaultBox(ctx *context.Context, mailIds []int, groupName string) error {
	switch groupName {
	case "Deleted Messages":
		return del_email.DelEmail(ctx, mailIds, false)
	case "INBOX":
		return del_email.MoveToDefault(ctx, mailIds, consts.EmailStatusWait)
	case "Sent Messages":
		return del_email.MoveToDefault(ctx, mailIds, consts.EmailStatusSent)
	case "Drafts":
		return del_email.MoveToDefault(ctx, mailIds, consts.EmailStatusDrafts)
	case "Junk":
		return del_email.MoveToDefault(ctx, mailIds, consts.EmailStatusJunk)
	}
	return nil
}
