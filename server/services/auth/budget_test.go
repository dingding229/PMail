package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestC7LookupBudgetLimitsEachIPAndTheProcess(t *testing.T) {
	b := &lookupBudget{}
	now := time.Now()
	for i := 0; i < sourceRate; i++ {
		release, ok := b.acquire("source", now)
		if !ok {
			t.Fatalf("blocked at %d", i)
		}
		release()
	}
	if _, ok := b.acquire("source", now); ok {
		t.Fatal("per-source rate not enforced")
	}
	for i := sourceRate; i < globalRate; i++ {
		release, ok := b.acquire(fmt.Sprint(i), now)
		if !ok {
			t.Fatalf("blocked at %d", i)
		}
		release()
	}
	if _, ok := b.acquire("new-source", now); ok {
		t.Fatal("global rate not enforced")
	}
	if release, ok := b.acquire("source", now.Add(budgetWindow)); !ok {
		t.Fatal("budget did not reset")
	} else {
		release()
	}
}
