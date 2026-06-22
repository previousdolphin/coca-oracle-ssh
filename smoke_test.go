package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Network smoke test: live skills fetch from GitHub + a real Anthropic call,
// proving the selected skill is what drives the answer. Guarded so it never
// runs in normal `go test`. Run with:
//
//	SMOKE=1 ANTHROPIC_API_KEY=sk-... go test -run TestSmoke -v
func TestSmokeSkillDrivesModel(t *testing.T) {
	if os.Getenv("SMOKE") == "" {
		t.Skip("set SMOKE=1 to run the network smoke test")
	}
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Fatal("ANTHROPIC_API_KEY required")
	}

	store := NewSkills(defaultSkillsBase)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Load(ctx); err != nil {
		t.Fatalf("load index: %v", err)
	}
	t.Logf("loaded %d skills", store.Count())

	oracle := NewOracle(key, os.Getenv("ORACLE_MODEL"))

	cases := []struct {
		skill, ask string
	}{
		{"inversion", "We plan to launch the website this Friday. Anything to consider?"},
		{"after-nietzsche", "Is honesty always good?"},
	}
	for _, c := range cases {
		sys, err := store.SystemFor(ctx, ModeGeneric, c.skill)
		if err != nil {
			t.Fatalf("system prompt %s: %v", c.skill, err)
		}
		if !strings.Contains(sys, "Apply the following skill") {
			t.Fatalf("glue line missing for %s", c.skill)
		}
		reply, err := oracle.Ask(ctx, sys, []Msg{{Role: "user", Content: c.ask}})
		if err != nil {
			t.Fatalf("ask under %s: %v", c.skill, err)
		}
		if strings.TrimSpace(reply) == "" {
			t.Fatalf("empty reply under %s", c.skill)
		}
		t.Logf("\n=== skill: %s ===\nQ: %s\nA: %s\n", c.skill, c.ask, reply)
	}
}
