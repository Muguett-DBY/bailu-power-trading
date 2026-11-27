package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	memoryDir        = ".bailu-memory"
	memoryIgnoreFile = "# Bailu project-local memory\n*\n"
)

type memoryRecord struct {
	Kind      string    `json:"kind"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Scope     string    `json:"scope"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

func memoryKind(kind string) bool {
	return kind == "preference" || kind == "correction"
}

func validMemoryText(field, value string, limit int) error {
	if value == "" || strings.ContainsRune(value, 0) || !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s需要 1 到 %d 个有效字符", field, limit)
	}
	return nil
}

func newMemoryRecord(o options, withValue bool) (memoryRecord, error) {
	var record memoryRecord
	count := 3
	if withValue {
		count++
	}
	if len(o.words) != count || !memoryKind(o.words[1]) {
		if withValue {
			return record, errors.New(`用法 bailu memory save preference|correction "键" "内容" [--scope 范围] [--source 来源]`)
		}
		return record, errors.New(`用法 bailu memory forget preference|correction "键" [--scope 范围]`)
	}
	record.Kind = o.words[1]
	record.Key = strings.TrimSpace(o.words[2])
	record.Scope = strings.TrimSpace(o.flag("scope", "project"))
	if err := validMemoryText("记忆键", record.Key, 120); err != nil {
		return record, err
	}
	if err := validMemoryText("适用范围", record.Scope, 120); err != nil {
		return record, err
	}
	if !withValue {
		return record, nil
	}
	record.Value = strings.TrimSpace(o.words[3])
	record.Source = strings.TrimSpace(o.flag("source", ""))
	if record.Source == "" {
		if record.Kind == "correction" {
			return record, errors.New("纠正记录需要 --source 注明由谁确认或来自哪份材料")
		}
		record.Source = "用户明确表达"
	}
	if err := validMemoryText("记忆内容", record.Value, 1200); err != nil {
		return record, err
	}
	return record, validMemoryText("来源", record.Source, 512)
}

func (m memoryRecord) filename() string {
	key := strings.ToLower(m.Kind + "\x00" + m.Scope + "\x00" + m.Key)
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]) + ".json"
}

func memoryProject(o options) (string, error) {
	if project, explicit := o.flags["project"]; explicit {
		return project, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for project := current; ; project = filepath.Dir(project) {
		if info, err := os.Stat(filepath.Join(project, ".bailu-install.json")); err == nil && info.Mode().IsRegular() {
			return project, nil
		}
		if filepath.Dir(project) == project {
			return current, nil
		}
	}
}

func openMemory(o options, create bool) (*os.Root, error) {
	project, err := memoryProject(o)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(project)
	if err != nil {
		return nil, fmt.Errorf("打开记忆所属项目 %q：%w", project, err)
	}
	defer root.Close()
	store, err := root.OpenRoot(memoryDir)
	if errors.Is(err, fs.ErrNotExist) {
		if !create {
			return nil, nil
		}
		if err := root.Mkdir(memoryDir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		store, err = root.OpenRoot(memoryDir)
	}
	if err != nil {
		return nil, fmt.Errorf("打开项目记忆目录：%w", err)
	}
	return store, nil
}

func readMemoryRecord(store *os.Root, name string) (memoryRecord, error) {
	var record memoryRecord
	data, err := store.ReadFile(name)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("读取记忆文件 %s：%w", name, err)
	}
	if !memoryKind(record.Kind) || record.filename() != name || record.UpdatedAt.IsZero() {
		return record, fmt.Errorf("记忆文件 %s 的标识或时间不正确", name)
	}
	return record, nil
}

func replaceMemoryFile(store *os.Root, name string, data []byte) error {
	temporary := ".write-" + rand.Text()
	file, err := store.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer store.Remove(temporary)
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return store.Rename(temporary, name)
}

func ensureMemoryIgnore(store *os.Root) error {
	ignore, err := store.ReadFile(".gitignore")
	if errors.Is(err, fs.ErrNotExist) {
		return replaceMemoryFile(store, ".gitignore", []byte(memoryIgnoreFile))
	}
	if err != nil {
		return err
	}
	if strings.ReplaceAll(string(ignore), "\r\n", "\n") != memoryIgnoreFile {
		return errors.New("项目记忆目录的 .gitignore 已修改，请恢复 Bailu 创建的规则（*）后再写入")
	}
	return nil
}

func saveMemory(store *os.Root, record memoryRecord, out io.Writer) error {
	if err := ensureMemoryIgnore(store); err != nil {
		return err
	}
	name := record.filename()
	old, err := readMemoryRecord(store, name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil && old.Value == record.Value && old.Source == record.Source {
		_, err = fmt.Fprintf(out, "项目记忆已一致 %s %q [%s]\n", record.Kind, record.Key, record.Scope)
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := replaceMemoryFile(store, name, append(data, '\n')); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "已写入项目记忆 %s %q [%s]\n", record.Kind, record.Key, record.Scope)
	return err
}

func (m memoryRecord) matches(kind, scope string, terms []string) bool {
	if kind != "" && kind != m.Kind || scope != "" && !strings.EqualFold(scope, m.Scope) {
		return false
	}
	haystack := strings.ToLower(m.Key + "\n" + m.Value + "\n" + m.Scope + "\n" + m.Source)
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func showMemory(o options, out io.Writer) error {
	if err := o.only("project", "scope", "kind", "limit", "format"); err != nil {
		return err
	}
	kind := o.flag("kind", "")
	if kind != "" && !memoryKind(kind) {
		return errors.New("--kind 接受 preference 或 correction")
	}
	format := o.flag("format", "markdown")
	if format != "markdown" && format != "json" {
		return errors.New("--format 接受 markdown 或 json")
	}
	limit, err := strconv.Atoi(o.flag("limit", "8"))
	if err != nil || limit < 0 || limit > 50 {
		return errors.New("--limit 需要 0 到 50 之间的整数，0 表示全部")
	}
	store, err := openMemory(o, false)
	if err != nil {
		return err
	}
	records := []memoryRecord{}
	if store != nil {
		defer store.Close()
		entries, err := fs.ReadDir(store.FS(), ".")
		if err != nil {
			return err
		}
		terms := strings.Fields(strings.ToLower(strings.Join(o.words[1:], " ")))
		scope := strings.TrimSpace(o.flag("scope", ""))
		for _, file := range entries {
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			record, err := readMemoryRecord(store, file.Name())
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue // The record was forgotten during this read.
				}
				return err
			}
			if record.matches(kind, scope, terms) {
				records = append(records, record)
			}
		}
	}
	// Files were listed by name, which also provides a stable order for timestamp ties.
	slices.SortStableFunc(records, func(a, b memoryRecord) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	total := len(records)
	if limit > 0 && total > limit {
		records = records[:limit]
	}
	if format == "json" {
		return json.NewEncoder(out).Encode(records)
	}
	var result strings.Builder
	if len(records) == 0 {
		result.WriteString("没有匹配的项目记忆。\n")
	} else {
		result.WriteString("以下是本项目的偏好和纠正记录。交易规则与电价的依据以适用的官方原文为准。\n")
		for _, record := range records {
			fmt.Fprintf(&result, "\n- %s [%q] %q → %q\n  来源 %q · %s\n", record.Kind, record.Scope,
				record.Key, record.Value, record.Source, record.UpdatedAt.Format(time.DateOnly))
		}
		if len(records) < total {
			fmt.Fprintf(&result, "\n显示了 %d 条，共 %d 条匹配。使用 --limit 0 查看全部。\n", len(records), total)
		}
	}
	_, err = io.WriteString(out, result.String())
	return err
}

func memoryCommand(o options, out io.Writer) error {
	if len(o.words) == 0 {
		return errors.New("请选择 bailu memory show、bailu memory save 或 bailu memory forget")
	}
	switch o.words[0] {
	case "show":
		return showMemory(o, out)
	case "save", "forget":
	default:
		return fmt.Errorf("不认识的项目记忆命令 %q", o.words[0])
	}
	save := o.words[0] == "save"
	allowed := []string{"project", "scope"}
	if save {
		allowed = append(allowed, "source")
	}
	if err := o.only(allowed...); err != nil {
		return err
	}
	record, err := newMemoryRecord(o, save)
	if err != nil {
		return err
	}
	store, err := openMemory(o, save)
	if err != nil {
		return err
	}
	if store != nil {
		defer store.Close()
		if save {
			return saveMemory(store, record, out)
		}
		if err := store.Remove(record.filename()); err == nil {
			_, err = fmt.Fprintf(out, "已遗忘项目记忆 %s %q [%s]\n", record.Kind, record.Key, record.Scope)
			return err
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "没有找到要遗忘的项目记忆 %s %q [%s]\n", record.Kind, record.Key, record.Scope)
	return err
}
