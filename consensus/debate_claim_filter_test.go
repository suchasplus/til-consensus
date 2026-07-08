package consensus

import "testing"

func TestStripDebateClaimStatusAffixes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailing chinese verdict from the tc_1783428965128 run",
			in:   "该方案时间复杂度O(n)，工程上只需维护用户级聚合表，SQL即可完成端到端计算。裁决：keep。",
			want: "该方案时间复杂度O(n)，工程上只需维护用户级聚合表，SQL即可完成端到端计算。",
		},
		{
			name: "trailing bracketed english status",
			in:   "Delta method fits realtime dashboards. [Status: keep]",
			want: "Delta method fits realtime dashboards.",
		},
		{
			name: "trailing parenthesized status with 保留",
			in:   "先做用户级聚合再检验。（裁决状态：保留）",
			want: "先做用户级聚合再检验。",
		},
		{
			name: "trailing final qualifier",
			in:   "结论成立。最终裁决：keep",
			want: "结论成立。",
		},
		{
			name: "prefix still stripped",
			in:   "[Status: revise] narrow the claim to mature cohorts",
			want: "narrow the claim to mature cohorts",
		},
		{
			name: "prefix and suffix together",
			in:   "裁决状态：keep。用户级聚合是推断基线。裁决：keep。",
			want: "用户级聚合是推断基线。",
		},
		{
			name: "status quo mention untouched",
			in:   "维持 status quo 更稳妥",
			want: "维持 status quo 更稳妥",
		},
		{
			name: "verdict followed by content untouched",
			in:   "最终裁决：保留原方案不变",
			want: "最终裁决：保留原方案不变",
		},
		{
			name: "pure status marker left for empty-statement handling",
			in:   "裁决：keep",
			want: "裁决：keep",
		},
		{
			name: "plain text untouched",
			in:   "Bootstrap 成本为 O(N×B)，适合离线复核",
			want: "Bootstrap 成本为 O(N×B)，适合离线复核",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripDebateClaimStatusAffixes(tc.in); got != tc.want {
				t.Fatalf("stripDebateClaimStatusAffixes(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
