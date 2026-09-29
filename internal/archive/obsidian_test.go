package archive

import "testing"

// TestSafeFolder 验证混合分隔符和上级目录片段会被安全处理。
func TestSafeFolder(t *testing.T) {
	if got := safeFolder("../Bilibili\\示例 UP 主"); got != "Bilibili/示例-UP-主" {
		t.Fatalf("safeFolder() = %q", got)
	}
}

// TestSafePart 验证标题中的非法文件名字符会替换为连字符。
func TestSafePart(t *testing.T) {
	if got := safePart(`标题: 一个/测试?`); got != "标题-一个-测试" {
		t.Fatalf("safePart() = %q", got)
	}
}
