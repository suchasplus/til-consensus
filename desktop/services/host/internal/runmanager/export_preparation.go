package runmanager

import (
	"fmt"
	"strings"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func writePreparationExport(b *strings.Builder, r *model.StoredRun) {
	if r.Snapshot.RequestedMode != "" {
		fmt.Fprintf(b, "\n## 模式选择与准备分析\n\n请求模式：%s；执行模式：%s。\n", r.Snapshot.RequestedMode, r.Snapshot.Mode)
	}
	if p := r.Snapshot.Preparation; p != nil {
		fmt.Fprintf(b, "\n准备分析：`%s`；输入 hash：`%s`。\n", p.ID, p.InputHash)
		if p.Source != nil {
			fmt.Fprintf(b, "\n分析来源：%s / %s；CLI版本：%s；选择ID：%s；实际报告模型：%s（来源：%s）。\n", p.Source.ProviderID, p.Source.CLIType, p.Source.CLIVersion, p.Source.RequestedModel, p.Source.ActualModel, p.Source.ModelSource)
			if p.Source.Mock {
				b.WriteString("\n此准备分析来自显式 Mock。\n")
			}
		}
		if p.Classification != nil {
			fmt.Fprintf(b, "\n建议：%s；信心：%.2f。%s\n", p.Classification.Recommendation, p.Classification.Confidence, p.Classification.Summary)
			for _, why := range p.Classification.Why {
				fmt.Fprintf(b, "- %s\n", why)
			}
			if r.Snapshot.RequestedMode != "auto" {
				b.WriteString("\n模式由用户显式选择；准备建议未覆盖用户选择。\n")
			}
		}
		if p.MaterialAnalysis != nil {
			fmt.Fprintf(b, "\n材料分析：%s\n", p.MaterialAnalysis.Summary)
			for _, gap := range p.MaterialAnalysis.Gaps {
				fmt.Fprintf(b, "- [%s] %s\n", gap.Importance, gap.Question)
			}
		}
	}
	if len(r.Snapshot.Materials) > 0 {
		b.WriteString("\n## 原文材料索引\n\n| 材料 | 来源 | 字节 | 内容 SHA-256 | 原始文件 SHA-256 |\n| --- | --- | ---: | --- | --- |\n")
		for _, m := range r.Snapshot.Materials {
			fmt.Fprintf(b, "| %s | %s | %d | `%s` | `%s` |\n", cell(m.Title), cell(m.Source), m.ByteLength, m.SHA256, m.RawSHA256)
		}
	}
}
