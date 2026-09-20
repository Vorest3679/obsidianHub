package archive

import "testing"

func TestSafeFolder(t *testing.T) {
	if got := safeFolder("../Bilibili\\示例 UP 主"); got != "Bilibili/示例-UP-主" {
		t.Fatalf("safeFolder() = %q", got)
	}
}

func TestSafePart(t *testing.T) {
	if got := safePart(`标题: 一个/测试?`); got != "标题-一个-测试" {
		t.Fatalf("safePart() = %q", got)
	}
}
