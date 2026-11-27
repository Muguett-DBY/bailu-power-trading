package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func runMemory(t *testing.T, project string, args ...string) string {
	t.Helper()
	command := []string{"memory", "--project", project}
	var out bytes.Buffer
	if err := run(append(command, args...), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func listMemory(t *testing.T, project string, args ...string) []memoryRecord {
	t.Helper()
	output := runMemory(t, project, append([]string{"show", "--format", "json"}, args...)...)
	var records []memoryRecord
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		t.Fatalf("读取记忆输出 %q：%v", output, err)
	}
	return records
}

func TestMemorySavesUpdatesFiltersAndForgetsProjectRecords(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	other := t.TempDir()
	if output := runMemory(t, project, "show", "--format", "json"); output != "[]\n" {
		t.Fatalf("一个新项目应返回空数组，实际是 %q", output)
	}
	if _, err := os.Stat(filepath.Join(project, memoryDir)); !os.IsNotExist(err) {
		t.Fatalf("空读取不应创建记忆目录：%v", err)
	}
	runMemory(t, project, "save", "preference", "月报格式", "使用 Word")
	first := listMemory(t, project, "--kind", "preference")
	if len(first) != 1 || first[0].Source != "用户明确表达" || first[0].Scope != "project" {
		t.Fatalf("默认来源和范围没有记下：%+v", first)
	}
	runMemory(t, project, "save", "preference", "月报格式", "使用 Word")
	same := listMemory(t, project, "月报")
	if len(same) != 1 || !same[0].UpdatedAt.Equal(first[0].UpdatedAt) {
		t.Fatalf("重复保存刷新了时间：%v", same)
	}
	runMemory(t, project, "save", "preference", "月报格式", "使用 Excel")
	runMemory(t, project, "save", "preference", "月报格式", "给运营组发 PDF", "--scope", "运营组")
	runMemory(t, project, "save", "correction", "月报格式", "天津结算月份取账单抬头", "--scope", "天津", "--source", "用户在本对话确认")
	updated := listMemory(t, project, "月报 Excel", "--scope", "project")
	if len(updated) != 1 || updated[0].Value != "使用 Excel" {
		t.Fatalf("应按关键词和范围找到更新后的项目偏好：%+v", updated)
	}
	corrections := listMemory(t, project, "账单", "--kind", "correction")
	if len(corrections) != 1 || corrections[0].Scope != "天津" || corrections[0].Source != "用户在本对话确认" {
		t.Fatalf("本地纠正应包含范围和证据：%+v", corrections)
	}
	if rows := listMemory(t, project, "--limit", "1"); len(rows) != 1 {
		t.Fatalf("结果未按 --limit 截取：%+v", rows)
	}
	if rows := listMemory(t, other); len(rows) != 0 {
		t.Fatalf("另一个项目读到了记忆：%+v", rows)
	}
	runMemory(t, project, "forget", "preference", "月报格式")
	if rows := listMemory(t, project); len(rows) != 2 || rows[0].Scope == "project" || rows[1].Scope == "project" {
		t.Fatalf("忘记某个项目偏好影响了其他范围：%+v", rows)
	}
}

func TestMemoryRejectsUnattributedCorrectionsBeforeWriting(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	err := run([]string{"memory", "save", "correction", "缺少来源", "某条纠正", "--project", project}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--source") {
		t.Fatalf("未拒绝缺少来源的纠正：%v", err)
	}
	if _, err := os.Stat(filepath.Join(project, memoryDir)); !os.IsNotExist(err) {
		t.Fatalf("输入不完整不应创建记忆目录：%v", err)
	}
}

func TestMemoryConcurrentSavesKeepDistinctRecords(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	const writers = 10
	const total = writers + 1
	errors := make(chan error, writers)
	var group sync.WaitGroup
	for i := range writers {
		group.Go(func() {
			name := fmt.Sprintf("偏好 %d", i)
			err := run([]string{"memory", "save", "preference", name, "保留工作底稿", "--project", project}, io.Discard)
			if err == nil {
				err = run([]string{"memory", "save", "preference", "共同偏好", name, "--project", project}, io.Discard)
			}
			errors <- err
		})
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if rows := listMemory(t, project, "--limit", "0"); len(rows) != total {
		t.Fatalf("并行保存后剩下 %d 条，预计 %d 条", len(rows), total)
	}
	if rows := listMemory(t, project, "共同偏好"); len(rows) != 1 || !strings.HasPrefix(rows[0].Value, "偏好 ") {
		t.Fatalf("并行更新同一记忆没有形成一条有效记录：%+v", rows)
	}
	if rows := listMemory(t, project); len(rows) != 8 {
		t.Fatalf("默认检索应该限制为 8 条，实际 %d 条", len(rows))
	}
	if output := runMemory(t, project, "show"); !strings.Contains(output, "共 11 条匹配") {
		t.Fatalf("未显示实际匹配条数：%s", output)
	}
}

func TestMemoryIgnoresRecordsInHostGitRepository(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	project := t.TempDir()
	if output, err := exec.Command(git, "-C", project, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("创建测试仓库：%v %s", err, output)
	}
	runMemory(t, project, "save", "preference", "版式", "保密测试输入")
	entries, err := filepath.Glob(filepath.Join(project, memoryDir, "*.json"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("缺少一条记忆文件：%v %v", entries, err)
	}
	name := filepath.ToSlash(filepath.Join(memoryDir, filepath.Base(entries[0])))
	output, err := exec.Command(git, "-C", project, "check-ignore", "-v", name).CombinedOutput()
	if err != nil || !strings.Contains(string(output), memoryDir+"/.gitignore") {
		t.Fatalf("测试仓库未采用记忆目录的忽略文件：%v %s", err, output)
	}
}

func TestMemoryDefaultsToDeploymentRootWithAnExplicitOverride(t *testing.T) {
	// The working directory is process-wide. Do not run this test in parallel.
	project := t.TempDir()
	nested := filepath.Join(project, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".bailu-install.json"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	if err := run([]string{"memory", "save", "preference", "输出", "先给结论"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if rows := listMemory(t, project); len(rows) != 1 || rows[0].Value != "先给结论" {
		t.Fatalf("没有写入部署根目录：%+v", rows)
	}
	if rows := listMemory(t, nested); len(rows) != 0 {
		t.Fatalf("显式项目路径不应读取父目录的记忆：%+v", rows)
	}
}
