// 锚点 slug 纯函数单测（M3-T03，设计 §9）：规则确定性、特殊字符、CJK 保留、
// 同页重复标题 -2/-3 后缀与基础 slug 碰撞避让。
package projection

import "testing"

func TestAnchorSlug(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"基本小写与空白折叠", "Hello World", "hello-world"},
		{"多重空白与前导尾随", "  Multiple   Spaces  ", "multiple-spaces"},
		{"连字符折叠", "a--b - c", "a-b-c"},
		{"CJK 保留不转写", "角色 安比", "角色-安比"},
		{"中日韩混合与数字", "第 3 章 Release2", "第-3-章-release2"},
		{"标点符号去除", "Foo & Bar! (v2)", "foo-bar-v2"},
		{"下划线等非字母数字去除", "snake_case_id", "snakecaseid"},
		{"大写折叠", "ABC Def", "abc-def"},
		{"全标点回落 section", "!!!", "section"},
		{"空串回落 section", "", "section"},
		{"纯空白回落 section", "   ", "section"},
		{"数字保留", "Section 2", "section-2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := anchorSlug(c.title); got != c.want {
				t.Fatalf("anchorSlug(%q) = %q, 期望 %q", c.title, got, c.want)
			}
		})
	}
}

func TestSlugAssignerDedup(t *testing.T) {
	a := newSlugAssigner()
	titles := []string{"Intro", "Intro", "Intro"}
	want := []string{"intro", "intro-2", "intro-3"}
	for i, title := range titles {
		if got := a.assign(title); got != want[i] {
			t.Fatalf("第 %d 次 assign(%q) = %q, 期望 %q", i+1, title, got, want[i])
		}
	}
}

// 基础 slug 恰好带数字后缀的标题与重复标题碰撞：确定性避让。
func TestSlugAssignerCollisionWithSuffixedBase(t *testing.T) {
	a := newSlugAssigner()
	titles := []string{"Intro", "Intro 2", "Intro"}
	// "Intro 2" 的基础 slug 即 intro-2，先占；第三个 "Intro" 顺延到 intro-3。
	want := []string{"intro", "intro-2", "intro-3"}
	for i, title := range titles {
		if got := a.assign(title); got != want[i] {
			t.Fatalf("第 %d 次 assign(%q) = %q, 期望 %q", i+1, title, got, want[i])
		}
	}
}

// 确定性：同一序列两个 assigner 产出相同结果。
func TestSlugAssignerDeterministic(t *testing.T) {
	titles := []string{"角色", "角色", "Story & Lore", "story lore"}
	a1, a2 := newSlugAssigner(), newSlugAssigner()
	for _, title := range titles {
		if s1, s2 := a1.assign(title), a2.assign(title); s1 != s2 {
			t.Fatalf("同序列 slug 不确定: %q vs %q（title %q）", s1, s2, title)
		}
	}
}
