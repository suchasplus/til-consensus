package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/internal/config"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

func TestDelegatePersistsParseErrorArtifact(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:     config.ProviderTypeMock,
				Behavior: "malformed",
				Models:   map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers:   []string{"proposer-a"},
			Challengers: []string{"proposer-a"},
		},
	}), tmp)
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, time.Second)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.OK {
		t.Fatal("expected malformed output to fail")
	}
	if awaited.Artifact == nil {
		t.Fatal("expected parse error artifact")
	}
	if _, err := os.Stat(awaited.Artifact.Path); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
}

func TestDelegateTimeoutReturnsTimeoutMarker(t *testing.T) {
	tmp := t.TempDir()
	delegate, err := NewDelegate(config.Normalize(config.Config{
		SchemaVersion: 1,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				Type:     config.ProviderTypeMock,
				Behavior: "timeout",
				Models:   map[string]config.ProviderModelConfig{"default": {ProviderModel: "mock"}},
			},
		},
		Agents: []config.AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default"},
		},
		Roles: config.RolesConfig{
			Proposers:   []string{"proposer-a"},
			Challengers: []string{"proposer-a"},
		},
	}), filepath.Join(tmp, "artifacts"))
	if err != nil {
		t.Fatalf("NewDelegate failed: %v", err)
	}
	receipt, err := delegate.Dispatch(context.Background(), consensus.ProposalTask{
		TaskMeta: consensus.TaskMeta{
			RequestID: "req-1",
			SessionID: "session-1",
			AgentID:   "proposer-a",
		},
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	awaited, err := delegate.Await(context.Background(), receipt.TaskID, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Await failed: %v", err)
	}
	if awaited.Error != "__timeout__" {
		t.Fatalf("expected timeout marker, got %#v", awaited)
	}
}
