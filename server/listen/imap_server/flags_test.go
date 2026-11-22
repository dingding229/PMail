package imap_server

import (
	"slices"
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/emersion/go-imap/v2"
)

func TestSeenFlaggedStoreClearAndReplace(t *testing.T) {
	for _, tc := range []struct {
		op       imap.StoreFlagsOp
		included bool
		change   bool
		want     bool
	}{
		{imap.StoreFlagsAdd, true, true, true}, {imap.StoreFlagsAdd, false, false, false},
		{imap.StoreFlagsDel, true, true, false}, {imap.StoreFlagsDel, false, false, false},
		{imap.StoreFlagsSet, true, true, true}, {imap.StoreFlagsSet, false, true, false},
	} {
		value := flagChange(tc.op, tc.included)
		if (value != nil) != tc.change || (value != nil && *value != tc.want) {
			t.Fatalf("case %+v: %v", tc, value)
		}
	}
	flags := mailFlags(true, true, consts.EmailStatusDrafts)
	if !slices.Equal(flags, []imap.Flag{imap.FlagSeen, imap.FlagFlagged, imap.FlagDraft}) {
		t.Fatalf("flags: %v", flags)
	}
	var set imap.SeqSet
	set.AddRange(2, 0)
	if !storeContains(set, 4, 99, 4, 105) || storeContains(set, 1, 90, 4, 105) {
		t.Fatal("bad wildcard resolution")
	}
}
