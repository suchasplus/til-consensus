package workflow

import (
	"encoding/json"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

// Snapshot contents are data. Paths are never opened and no ingest command is
// derived from material text. Keep only explicit provenance in task metadata.
func MaterialRefs(materials []model.Material) []consensus.MaterialRef {
	refs := make([]consensus.MaterialRef, 0, len(materials))
	for _, m := range materials {
		body, _ := json.Marshal(m)
		refs = append(refs, consensus.MaterialRef{ID: m.ID, Title: m.Title, Kind: "source_material", Content: string(body), Hash: Hash(string(body)), Metadata: map[string]any{"snapshotFormat": "tcview-material-v1", "source": m.Source, "sourceSha256": m.SHA256, "rawSha256": m.RawSHA256, "byteLength": m.ByteLength, "normalization": m.Normalization}})
	}
	return refs
}
func P0Materials(r model.StoredRun) []consensus.MaterialRef {
	refs := MaterialRefs(r.Snapshot.Materials)
	if r.ParentMaterial != "" {
		refs = append(refs, consensus.MaterialRef{ID: r.Snapshot.ParentRunID + "-result", Title: "父讨论冻结结果（参考，旧投票无效）", Kind: "reference", Content: r.ParentMaterial, Hash: Hash(r.ParentMaterial)})
	}
	if r.Snapshot.Mode == "delphi" {
		for i := range refs {
			refs[i] = anonymousMaterial(refs[i])
		}
	}
	return refs
}
